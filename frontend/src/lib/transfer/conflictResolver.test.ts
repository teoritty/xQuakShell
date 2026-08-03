import { resolveConflicts, type ConflictDecision } from './conflictResolver';
import type { PlannedFileDTO } from '../../backend/gateway';

function assert(cond: boolean, msg: string): void {
  if (!cond) throw new Error(msg);
}

function conflict(target: string): PlannedFileDTO {
  return { source: '/s/' + target, target, size: 1, srcModTime: '', conflict: { size: 2, modTime: '', isDir: false } };
}

const decision = (over: Partial<ConflictDecision>): ConflictDecision => ({
  action: 'overwrite',
  applyToAll: false,
  rememberDefault: false,
  ...over,
});

// countingPrompt records how many times it was invoked.
function countingPrompt(fn: () => Promise<ConflictDecision | null>) {
  const wrapper = async (): Promise<ConflictDecision | null> => {
    wrapper.calls++;
    return fn();
  };
  wrapper.calls = 0;
  return wrapper;
}

async function main(): Promise<void> {
  // No conflicts → empty resolutions, prompt never called.
  {
    const prompt = countingPrompt(async () => decision({}));
    const res = await resolveConflicts([], 'ask', prompt);
    assert(res !== null && res.resolutions.length === 0 && res.persistDefault === null, 'empty → empty result');
    assert(prompt.calls === 0, 'empty → no prompt');
  }

  // Persisted default applies to all conflicts without prompting.
  {
    const prompt = countingPrompt(async () => decision({}));
    const res = await resolveConflicts([conflict('/a'), conflict('/b')], 'skip', prompt);
    assert(prompt.calls === 0, 'default → no prompt');
    assert(
      !!res &&
        res.resolutions.length === 2 &&
        res.resolutions.every((r) => r.action === 'skip'),
      'default applied to all',
    );
  }

  // Ask default prompts each conflict.
  {
    const prompt = countingPrompt(async () => decision({ action: 'overwrite' }));
    const res = await resolveConflicts([conflict('/a'), conflict('/b')], 'ask', prompt);
    assert(prompt.calls === 2, 'ask → prompt each');
    assert(!!res && res.resolutions.map((r) => r.action).join(',') === 'overwrite,overwrite', 'both overwrite');
  }

  // "Always use this action" resolves the rest without prompting.
  {
    const prompt = countingPrompt(async () => decision({ action: 'skip', applyToAll: true }));
    const res = await resolveConflicts([conflict('/a'), conflict('/b'), conflict('/c')], 'ask', prompt);
    assert(prompt.calls === 1, 'applyToAll → single prompt');
    assert(!!res && res.resolutions.length === 3 && res.resolutions.every((r) => r.action === 'skip'), 'sticky applied');
  }

  // Cancelling any prompt aborts the batch (null).
  {
    const prompt = countingPrompt(async () => null);
    const res = await resolveConflicts([conflict('/a')], 'ask', prompt);
    assert(res === null, 'cancel → null');
  }

  // rememberDefault surfaces the action to persist.
  {
    const prompt = countingPrompt(async () => decision({ action: 'overwrite_if_newer', rememberDefault: true, applyToAll: true }));
    const res = await resolveConflicts([conflict('/a')], 'ask', prompt);
    assert(!!res && res.persistDefault === 'overwrite_if_newer', 'persistDefault surfaced');
  }

  // An explicit rename name is kept only for that file.
  {
    let call = 0;
    const prompt = countingPrompt(async () => {
      call++;
      return call === 1 ? decision({ action: 'rename', newName: 'copy.txt' }) : decision({ action: 'skip' });
    });
    const res = await resolveConflicts([conflict('/a'), conflict('/b')], 'ask', prompt);
    assert(!!res && res.resolutions[0].newName === 'copy.txt', 'explicit rename name kept');
    assert(!!res && res.resolutions[1].action === 'skip' && res.resolutions[1].newName === undefined, 'second uses own decision');
  }

  console.log('conflictResolver.test passed');
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
