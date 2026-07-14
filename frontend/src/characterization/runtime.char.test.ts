import { setGateway, setRuntime } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import type { RuntimeGateway } from '../backend/gateway';
import {
  subscribeToEvents,
  sftpReadyPaths,
  getSettings,
  uploadFile,
  connectionProtocolCatalogKey,
  registerTerminalOutputConsumer,
  takePendingTerminalOutput,
} from '../stores/api';
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
    id: 't1', sessionId: 's1', kind: 'delete', direction: 'upload',
    localPath: '', remotePath: '/x', done: 0, total: 0, state: 'cancelled',
  };
  handlers.get('TransferProgress')!(del);
  // isOp (delete/chmod/chown) + terminal state (cancelled/failed/completed) => refresh signal.
  assert(get(transferCompleted)?.id === 't1', 'delete-kind cancelled transfer sets transferCompleted'); // api.ts:1022-1033
}

{
  reset();
  const up: TransferItem = {
    id: 't2', sessionId: 's1', kind: 'upload', direction: 'upload',
    localPath: '/a', remotePath: '/b', done: 0, total: 0, state: 'cancelled',
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

// --- connectionProtocolCatalogKey ------------------------------------------

{
  const key = connectionProtocolCatalogKey([{ id: 'ssh', fields: [] } as any]);
  assert(key === 'ssh:0', 'connectionProtocolCatalogKey formats as "<id>:<fieldCount>" joined by "|"'); // api.ts:1168-1174
}

console.log('runtime.char.test passed');
