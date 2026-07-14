import { setRuntime } from '../backend/context';
import type { RuntimeGateway } from '../backend/gateway';
import { subscribeToEvents, sftpReadyPaths } from './subscribe';
import {
  sessions,
  transferCompleted,
  vaultUnlocked,
  folders,
  connections,
  identities,
  type Session,
  type TransferItem,
} from '../stores/appState';
import { registerTerminalOutputConsumer, takePendingTerminalOutput } from '../terminal/outputBuffer';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

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
  vaultUnlocked.set(true);
  folders.set([{ id: 'f1', name: 'F', parentId: '', order: 0 }]);
  connections.set([{ id: 'c1', folderId: '', name: 'C', host: 'h', port: 22, order: 0 }]);
  identities.set([{ id: 'i1', name: 'I' } as any]);
}

const { runtime, handlers } = createFakeRuntime();
setRuntime(runtime);
subscribeToEvents();

// --- SFTPReady latching --------------------------------------------------

{
  reset();
  handlers.get('SFTPReady')!({ sessionId: 's1', initialPath: '/home' });
  assert(get(sftpReadyPaths).get('s1') === '/home', 'SFTPReady latches path');
}

{
  reset();
  handlers.get('SFTPReady')!({ sessionId: 's1' });
  assert(get(sftpReadyPaths).get('s1') === '/', 'SFTPReady with no initialPath latches "/"');
}

// --- SessionStateChanged closed cleanup ----------------------------------

{
  reset();
  sessions.set([{ sessionId: 's1', connectionId: 'c1', connectionName: 'C', state: 'ready', errorMessage: '' }]);
  sftpReadyPaths.set(new Map([['s1', '/home']]));
  handlers.get('SessionStateChanged')!({ sessionId: 's1', state: 'closed' } as Session);
  assert(get(sessions).length === 0, 'SessionStateChanged closed removes the session');
  assert(!get(sftpReadyPaths).has('s1'), 'SessionStateChanged closed clears the sftpReadyPaths entry');
}

// --- TransferProgress: op-vs-byte distinction ----------------------------

{
  reset();
  const del: TransferItem = {
    id: 't1', sessionId: 's1', kind: 'delete', direction: 'upload',
    localPath: '', remotePath: '/x', done: 0, total: 0, state: 'cancelled',
  };
  handlers.get('TransferProgress')!(del);
  assert(get(transferCompleted)?.id === 't1', 'delete-kind cancelled transfer sets transferCompleted');
}

{
  reset();
  const up: TransferItem = {
    id: 't2', sessionId: 's1', kind: 'upload', direction: 'upload',
    localPath: '/a', remotePath: '/b', done: 0, total: 0, state: 'cancelled',
  };
  handlers.get('TransferProgress')!(up);
  assert(get(transferCompleted) === null, 'upload-kind cancelled transfer does NOT set transferCompleted');
}

// --- TerminalOutput: consumer-gated buffering ----------------------------

{
  reset();
  const b64 = btoa('hello');
  const release = registerTerminalOutputConsumer('s1');
  handlers.get('TerminalOutput')!({ sessionId: 's1', output: b64 });
  assert(takePendingTerminalOutput('s1').length === 0, 'TerminalOutput is NOT buffered while a consumer is registered');
  release();

  handlers.get('TerminalOutput')!({ sessionId: 's1', output: b64 });
  const chunks = takePendingTerminalOutput('s1');
  assert(chunks.length === 1, 'TerminalOutput IS buffered when no consumer is registered');
  assert(new TextDecoder().decode(chunks[0]) === 'hello', 'buffered chunk decodes back to the original bytes');
}

// --- VaultLocked: 5-store clear ------------------------------------------

{
  reset();
  handlers.get('VaultLocked')!(undefined);
  assert(get(vaultUnlocked) === false, 'VaultLocked sets vaultUnlocked to false');
  assert(get(folders).length === 0, 'VaultLocked clears folders');
  assert(get(connections).length === 0, 'VaultLocked clears connections');
  assert(get(sessions).length === 0, 'VaultLocked clears sessions');
  assert(get(identities).length === 0, 'VaultLocked clears identities');
}

// --- SessionEmbedReady ----------------------------------------------------

{
  reset();
  sessions.set([{ sessionId: 's1', connectionId: 'c1', connectionName: 'C', state: 'ready', errorMessage: '' }]);
  handlers.get('SessionEmbedReady')!({ sessionId: 's1', embed: { kind: 'x' } as any });
  const s = get(sessions)[0] as any;
  assert(s.surface === 'embed', 'SessionEmbedReady sets surface to embed');
  assert(s.embed?.kind === 'x', 'SessionEmbedReady attaches the embed payload');
}

// --- SessionClosed ----------------------------------------------------

{
  reset();
  sessions.set([{ sessionId: 's1', connectionId: 'c1', connectionName: 'C', state: 'ready', errorMessage: '' }]);
  sftpReadyPaths.set(new Map([['s1', '/home']]));
  registerTerminalOutputConsumer('s1')();
  handlers.get('SessionClosed')!({ sessionId: 's1' });
  assert(get(sessions).length === 0, 'SessionClosed removes the session');
  assert(!get(sftpReadyPaths).has('s1'), 'SessionClosed clears the sftpReadyPaths entry');
}

// --- HostKeyRequired / PingUpdated presence -------------------------------

assert(handlers.has('HostKeyRequired'), 'HostKeyRequired handler registered');
assert(handlers.has('PingUpdated'), 'PingUpdated handler registered');
assert(handlers.has('FileEdited'), 'FileEdited handler registered');

console.log('subscribe.test passed');
