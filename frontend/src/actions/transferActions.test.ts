// Lifecycle invariant on the frontend side: the backend owns the Transfers-panel
// item. Once the planner has published one under plan.opID, the frontend must
// never delete it locally — it either lets the transfer run (the backend closes
// the item) or asks the backend to close it via CancelTransfer. These tests pin
// that ownership down, because the failure mode it replaces is invisible: an
// item stuck on "Scanning…" until the app restarts.
import { setGateway } from '../backend/context';
import { createFakeGateway, type FakeGateway } from '../backend/fakeGateway';
import { startLocalCopyDrop } from './transferActions';
import { transfers, lastError } from '../stores/appState';
import { conflictRequest, respondConflict } from '../stores/conflictPrompt';
import type { TransferPlanDTO } from '../backend/gateway';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

function reset() {
  transfers.set([]);
  lastError.set(null);
}

function planWith(files: TransferPlanDTO['files']): TransferPlanDTO {
  return { kind: 'localcopy', opID: files.length ? 'op-1' : '', dirs: [], files };
}

const CLEAN_FILE = { source: '/src/a.txt', target: '/dst/a.txt', size: 7, srcModTime: '' };
const CONFLICT_FILE = {
  source: '/src/a.txt',
  target: '/dst/a.txt',
  size: 7,
  srcModTime: '',
  conflict: { name: 'a.txt', size: 3, modTime: '', isDir: false },
};

// The live scan item as subscribe.ts would have created it from the planner's
// first event. Its survival is the observable proof that the frontend stopped
// deleting items it does not own.
function seedScanItem(id = 'op-1') {
  transfers.set([{ id, state: 'active', progress: 0 } as any]);
}

function called(fake: FakeGateway, method: string) {
  return fake.calls.filter((c) => c.method === method);
}

// Answers the conflict dialog as soon as the resolver opens it. Returns an
// unsubscribe so a test's answer never leaks into the next one.
function answerConflictWith(decision: unknown) {
  return conflictRequest.subscribe((req) => {
    if (!req) return;
    // Defer: respondConflict writes conflictRequest, and re-entering a store
    // subscriber mid-write is exactly the kind of thing that hangs a test.
    queueMicrotask(() => respondConflict(decision as any));
  });
}

// A settings object whose default is 'ask', so a conflict actually prompts.
const ASK_SETTINGS = { defaultUploadExistsAction: 'ask', defaultDownloadExistsAction: 'ask' };

async function run() {
  // --- empty plan -----------------------------------------------------------

  // The planner already closed an empty plan as completed (finishPlan branch 2)
  // and handed back no op id. The frontend must do nothing at all: no execute,
  // and above all no local deletion of a panel item it does not own.
  {
    reset();
    seedScanItem();
    const fake = createFakeGateway();
    fake.program('PlanLocalCopy', planWith([]));
    setGateway(fake);

    await startLocalCopyDrop(['/src'], '/dst');

    assert(called(fake, 'ExecuteLocalCopy').length === 0, 'empty plan does not execute');
    assert(called(fake, 'CancelTransfer').length === 0, 'empty plan does not cancel: the planner already closed the item');
    assert(get(transfers).length === 1, 'empty plan leaves the panel item alone — the backend owns it');
  }

  // --- clean plan -----------------------------------------------------------

  // No conflicts: straight to execute, and the item is claimed by the executor.
  {
    reset();
    seedScanItem();
    const fake = createFakeGateway();
    fake.program('PlanLocalCopy', planWith([CLEAN_FILE]));
    fake.program('ExecuteLocalCopy', undefined);
    setGateway(fake);

    await startLocalCopyDrop(['/src/a.txt'], '/dst');

    const exec = called(fake, 'ExecuteLocalCopy');
    assert(exec.length === 1, 'clean plan executes exactly once');
    const req = exec[0].args[0] as { plan: TransferPlanDTO; resolutions: unknown[] };
    assert(req.plan.opID === 'op-1', 'execute carries the planned op id so both phases share one item');
    assert(req.resolutions.length === 0, 'a clean plan needs no resolutions');
    assert(called(fake, 'CancelTransfer').length === 0, 'an executed plan is not also cancelled');
    assert(get(transfers).length === 1, 'the panel item survives — its terminal event arrives from the backend');
  }

  // --- user cancels the conflict dialog ------------------------------------

  // The regression this file exists for. The old code called removeTransfer()
  // and returned, leaving the backend's registration alive and the item with no
  // terminal event. Now the frontend asks the backend to close it and leaves the
  // list untouched.
  {
    reset();
    seedScanItem();
    const fake = createFakeGateway();
    fake.program('PlanLocalCopy', planWith([CONFLICT_FILE]));
    fake.program('GetSettings', ASK_SETTINGS);
    fake.program('CancelTransfer', undefined);
    setGateway(fake);
    const unsub = answerConflictWith(null); // user hit Cancel

    await startLocalCopyDrop(['/src/a.txt'], '/dst');
    unsub();

    assert(called(fake, 'ExecuteLocalCopy').length === 0, 'a cancelled batch does not execute');
    const cancels = called(fake, 'CancelTransfer');
    assert(cancels.length === 1, 'a cancelled batch closes the item through the backend');
    assert(cancels[0].args[0] === 'op-1', 'CancelTransfer targets the plan op id');
    assert(get(transfers).length === 1, 'the frontend does not delete the item it asked the backend to close');
  }

  // --- persisting the "remember my choice" default -------------------------

  // A side write of settings must never stand between a confirmed transfer and
  // its execution: the user already said yes.
  {
    reset();
    seedScanItem();
    const fake = createFakeGateway();
    fake.program('PlanLocalCopy', planWith([CONFLICT_FILE]));
    fake.program('GetSettings', ASK_SETTINGS);
    fake.program('SaveSettings', () => { throw new Error('disk full'); });
    fake.program('ExecuteLocalCopy', undefined);
    setGateway(fake);
    const unsub = answerConflictWith({ action: 'overwrite', applyToAll: false, rememberDefault: true });

    await startLocalCopyDrop(['/src/a.txt'], '/dst');
    unsub();

    assert(called(fake, 'SaveSettings').length === 1, 'the default was attempted');
    assert(called(fake, 'ExecuteLocalCopy').length === 1, 'a failed settings write does not abort the confirmed transfer');
    assert(called(fake, 'CancelTransfer').length === 0, 'the transfer ran, so the item is not cancelled');
  }

  // --- the execute RPC itself fails ----------------------------------------

  // ExecuteUpload/Download can fail *before* ExecutePlan runs (no session
  // context, no backend), in which case nothing emitted a terminal event. The
  // banner alone is not enough: the item must be handed back to be closed.
  {
    reset();
    seedScanItem();
    const fake = createFakeGateway();
    fake.program('PlanLocalCopy', planWith([CLEAN_FILE]));
    fake.program('ExecuteLocalCopy', () => { throw new Error('session not found'); });
    fake.program('CancelTransfer', undefined);
    setGateway(fake);

    await startLocalCopyDrop(['/src/a.txt'], '/dst');

    assert(get(lastError) !== null, 'a failing execute RPC still raises the banner');
    const cancels = called(fake, 'CancelTransfer');
    assert(cancels.length === 1 && cancels[0].args[0] === 'op-1', 'an unclaimed item is handed back to the backend to close');
  }

  console.log('transferActions.test passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
