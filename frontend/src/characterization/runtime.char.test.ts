import { setGateway, setRuntime } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import type { RuntimeGateway } from '../backend/gateway';
import { subscribeToEvents, sftpReadyPaths } from '../events/subscribe';
import {
  getSettings,
  saveSettings,
  applyAppearanceSettings,
} from '../actions/settingsActions';
import { uploadFile } from '../api/remoteFs';
import { connectionProtocolCatalogKey } from '../actions/protocolActions';
import {
  registerTerminalOutputConsumer,
  takePendingTerminalOutput,
} from '../terminal/outputBuffer';
import {
  sessions,
  transferCompleted,
  lastError,
  vaultUnlocked,
  folders,
  connections,
  identities,
  type Session,
  type TransferItem,
} from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

// Fake RuntimeGateway: records the handler registered for each event name so
// tests can fire it directly with a synthetic payload, mirroring how the real
// Wails runtime would invoke it.
function createFakeRuntime() {
  const handlers = new Map<string, (data: any) => void>();
  const runtime: RuntimeGateway = {
    EventsOn(event: string, cb: (data: any) => void) {
      handlers.set(event, cb);
    },
  };
  return { runtime, handlers };
}

function reset() {
  sessions.set([]);
  sftpReadyPaths.set(new Map());
  transferCompleted.set(null as unknown as TransferItem);
  lastError.set(null);
  vaultUnlocked.set(true);
  folders.set([{ id: 'f1', name: 'F', parentId: '', order: 0 }]);
  connections.set([{ id: 'c1', folderId: '', name: 'C', host: 'h', port: 22, order: 0 }]);
  identities.set([{ id: 'i1', name: 'I' } as any]);
}

const { runtime, handlers } = createFakeRuntime();
setRuntime(runtime);
setGateway(createFakeGateway());
subscribeToEvents(); // api.ts:950-1073 registers all handlers below via rt.EventsOn

// --- SFTPReady ---------------------------------------------------------

{
  reset();
  handlers.get('SFTPReady')!({ sessionId: 's1' }); // no initialPath
  assert(get(sftpReadyPaths).get('s1') === '/', 'SFTPReady with no initialPath latches "/" '); // api.ts:958
}

// --- SessionStateChanged -------------------------------------------------

{
  reset();
  sessions.set([{ sessionId: 's1', connectionId: 'c1', connectionName: 'C', state: 'ready', errorMessage: '' }]);
  sftpReadyPaths.set(new Map([['s1', '/home']]));
  handlers.get('SessionStateChanged')!({ sessionId: 's1', state: 'closed' } as Session);
  assert(get(sessions).length === 0, 'SessionStateChanged closed removes the session'); // api.ts:974-976
  assert(!get(sftpReadyPaths).has('s1'), 'SessionStateChanged closed clears the sftpReadyPaths entry'); // api.ts:965-970
}

// --- TransferProgress: delete-kind vs upload-kind terminal states --------

{
  reset();
  const del: TransferItem = {
    id: 't1', sessionId: 's1', kind: 'delete',
    localPath: '', remotePath: '/x', refreshDir: '/', done: 0, total: 0, state: 'cancelled',
  };
  handlers.get('TransferProgress')!(del);
  // isOp (delete/chmod/chown) + terminal state (cancelled/failed/completed) => refresh signal.
  assert(get(transferCompleted)?.id === 't1', 'delete-kind cancelled transfer sets transferCompleted'); // api.ts:1022-1033
}

{
  reset();
  const up: TransferItem = {
    id: 't2', sessionId: 's1', kind: 'upload',
    localPath: '/a', remotePath: '/b', refreshDir: '/', done: 0, total: 0, state: 'cancelled',
  };
  handlers.get('TransferProgress')!(up);
  // upload is not an "op" kind, and state !== 'completed', so no refresh signal.
  assert(get(transferCompleted) === null, 'upload-kind cancelled transfer does NOT set transferCompleted'); // api.ts:1022-1024
}

// --- TerminalOutput: buffered only when no consumer is registered --------

{
  reset();
  const b64 = btoa('hello');
  const release = registerTerminalOutputConsumer('s1'); // api.ts (re-exported from terminal/outputBuffer)
  handlers.get('TerminalOutput')!({ sessionId: 's1', output: b64 });
  assert(takePendingTerminalOutput('s1').length === 0, 'TerminalOutput is NOT buffered while a consumer is registered'); // api.ts:988
  release();

  handlers.get('TerminalOutput')!({ sessionId: 's1', output: b64 });
  const chunks = takePendingTerminalOutput('s1');
  assert(chunks.length === 1, 'TerminalOutput IS buffered when no consumer is registered'); // api.ts:986-989
  assert(new TextDecoder().decode(chunks[0]) === 'hello', 'buffered chunk decodes back to the original bytes');
}

// --- VaultLocked -----------------------------------------------------

{
  reset();
  handlers.get('VaultLocked')!(undefined);
  assert(get(vaultUnlocked) === false, 'VaultLocked sets vaultUnlocked to false'); // api.ts:1052
  assert(get(folders).length === 0, 'VaultLocked clears folders'); // api.ts:1053
  assert(get(connections).length === 0, 'VaultLocked clears connections'); // api.ts:1054
  assert(get(sessions).length === 0, 'VaultLocked clears sessions'); // api.ts:1055
  assert(get(identities).length === 0, 'VaultLocked clears identities'); // api.ts:1056
}

// --- getSettings: "vault is locked" is silenced ---------------------------

{
  reset();
  const fake = createFakeGateway();
  fake.program('GetSettings', () => {
    throw new Error('vault is locked');
  });
  setGateway(fake);
  const result = await getSettings();
  assert(result === null, 'getSettings returns null when the vault is locked'); // api.ts:836-841
  assert(get(lastError) === null, 'getSettings does NOT populate lastError for the expected "vault is locked" error'); // api.ts:838-841 (handleError is skipped on this branch)
}

// --- uploadFile: cancellation errors are silenced --------------------------

{
  reset();
  const fake = createFakeGateway();
  fake.program('Upload', () => {
    throw new Error('Upload CANCELLED by user');
  });
  setGateway(fake);
  await uploadFile('s1', '/local', '/remote');
  // isCancelError does e.message.toLowerCase().includes('cancel') -- case-insensitive substring match.
  assert(get(lastError) === null, 'uploadFile does not populate lastError when the error message contains "cancel" (any case)'); // api.ts:488-499
}

// --- saveSettings -----------------------------------------------------------

// api.ts:847-862 saveSettings: normalizes all four session hotkeys (falling
// back to DEFAULT_SESSION_HOTKEYS when unset) into the payload sent to
// SaveSettings, spreading the rest of `settings` through unchanged.
{
  reset();
  const fake = createFakeGateway();
  fake.program('SaveSettings', undefined);
  setGateway(fake);

  await saveSettings({ theme: 'dark' } as any); // hotkeys omitted -> defaults used
  const call = fake.calls.find(c => c.method === 'SaveSettings');
  const payload = call?.args[0] as any;
  assert(payload.theme === 'dark', 'saveSettings passes through other settings fields unchanged'); // api.ts:851-852
  assert(
    typeof payload.sessionHotkeyCreate === 'string' && payload.sessionHotkeyCreate.length > 0,
    'saveSettings fills in a normalized sessionHotkeyCreate when none was supplied',
  ); // api.ts:853
}

// saveSettings: RPC failure is swallowed via handleError (no rethrow).
{
  reset();
  const fake = createFakeGateway();
  fake.program('SaveSettings', () => { throw new Error('save failed'); });
  setGateway(fake);

  let threw: unknown = null;
  try {
    await saveSettings({});
  } catch (e) {
    threw = e;
  }
  assert(threw === null, 'saveSettings does not rethrow on RPC failure'); // api.ts:859-861
  assert(get(lastError)?.message === 'Save settings: save failed', 'saveSettings reports RPC failure via handleError'); // api.ts:860
}

// --- applyAppearanceSettings --------------------------------------------------

// api.ts:864-868 applyAppearanceSettings: fetches settings via getSettings and,
// when null (e.g. app absent), short-circuits without touching uiScale/document.
{
  reset();
  setGateway(null);
  let threw: unknown = null;
  try {
    await applyAppearanceSettings();
  } catch (e) {
    threw = e;
  }
  assert(threw === null, 'applyAppearanceSettings is a no-op (does not throw) when getSettings returns null'); // api.ts:865-866
}

// applyAppearanceSettings: with a settings object present, it proceeds past
// the `if (!s) return;` guard and calls applyUiScalePercent(s.uiScalePercent
// ?? DEFAULT_UI_SCALE_PERCENT). In a real browser `document`/`window` always
// exist, so this path never throws; we stub the handful of DOM entry points
// applyUiScalePercent actually touches (documentElement.style.{removeProperty,
// setProperty}, document.querySelectorAll, window.dispatchEvent) so the test
// can characterize the real fallback/propagation logic instead of pinning a
// Node-only "document is undefined" crash. `MutationObserver` is left
// undefined on purpose: applyUiScalePercent's `ensureIconObserver` already
// guards on `typeof MutationObserver === 'undefined'` and no-ops, so this
// matches real behavior in that (rare) environment without adding an
// unrelated stub.
{
  reset();
  const fake = createFakeGateway();
  fake.program('GetSettings', { uiScalePercent: 120 });
  setGateway(fake);

  const previousDocument = (globalThis as any).document;
  const previousWindow = (globalThis as any).window;
  const styleProps = new Map<string, string>();
  const dispatched: any[] = [];
  (globalThis as any).document = {
    documentElement: {
      style: {
        removeProperty: (k: string) => styleProps.delete(k),
        setProperty: (k: string, v: string) => styleProps.set(k, v),
      },
    },
    querySelectorAll: () => [] as unknown as NodeListOf<Element>,
  };
  (globalThis as any).window = {
    dispatchEvent: (e: unknown) => {
      dispatched.push(e);
      return true;
    },
  };

  let threw: unknown = null;
  try {
    await applyAppearanceSettings();
  } catch (e) {
    threw = e;
  } finally {
    (globalThis as any).document = previousDocument;
    (globalThis as any).window = previousWindow;
  }

  assert(threw === null, 'applyAppearanceSettings with a non-null settings object does not throw when document/window are available'); // api.ts:864-868
  assert(styleProps.get('--ui-scale') === '1.2', 'applyAppearanceSettings propagates uiScalePercent (120) into the --ui-scale CSS custom property (120/100)'); // api.ts:867, uiScale.ts:79-83
  assert(dispatched.length === 2 && dispatched[1]?.detail?.percent === 120, 'applyAppearanceSettings drives applyUiScalePercent to dispatch a resize and a ui-scale-changed event carrying the normalized percent'); // uiScale.ts:87-92
}

// --- connectionProtocolCatalogKey ------------------------------------------

{
  const key = connectionProtocolCatalogKey([{ id: 'ssh', fields: [] } as any]);
  assert(key === 'ssh:0', 'connectionProtocolCatalogKey formats as "<id>:<fieldCount>" joined by "|"'); // api.ts:1168-1174
}

console.log('runtime.char.test passed');
