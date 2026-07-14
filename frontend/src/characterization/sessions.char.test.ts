import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  openSession,
  closeSession,
  closeActiveSession,
  createSessionFromSelection,
  focusNextSessionTab,
  focusPrevSessionTab,
} from '../stores/api';
import { sessions, activeSessionId, connections, selectedConnectionId, lastError } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

function reset() {
  sessions.set([]);
  activeSessionId.set('');
  connections.set([]);
  selectedConnectionId.set('');
  lastError.set(null);
}

// --- openSession -----------------------------------------------------

// api.ts:329-356 openSession: calls app.OpenSession, then optimistically
// pushes a tab with state 'connecting' and sets activeSessionId.
{
  reset();
  connections.set([{ id: 'c1', folderId: '', name: 'MyConn', host: 'h', port: 22, order: 0 }]);
  const fake = createFakeGateway();
  fake.program('OpenSession', 'sess-1');
  setGateway(fake);

  const result = await openSession('c1');
  assert(result === 'sess-1', 'openSession returns the sessionId from the RPC'); // api.ts:333,351
  const list = get(sessions);
  assert(list.length === 1, 'openSession adds exactly one optimistic tab');
  assert(
    list[0].sessionId === 'sess-1' &&
      list[0].connectionId === 'c1' &&
      list[0].connectionName === 'MyConn' &&
      list[0].protocol === 'ssh' &&
      list[0].state === 'connecting' &&
      list[0].errorMessage === '',
    'optimistic tab shape matches api.ts:340-349',
  ); // api.ts:340-349 (protocol defaults to 'ssh' when conn.protocol is unset, connectionName falls back to conn?.name)
  assert(get(activeSessionId) === 'sess-1', 'openSession sets activeSessionId to the new session id'); // api.ts:350

  // Calling again with a connectionId that maps to the same returned sessionId
  // (the fake always returns 'sess-1' for OpenSession) does not duplicate the tab.
  await openSession('c1');
  assert(get(sessions).length === 1, 'repeat openSession with same resulting sessionId does not duplicate the tab'); // api.ts:337
}

// openSession when connection is not found in `connections`: falls back to
// connectionName 'Session' and protocol 'ssh'.
{
  reset();
  const fake = createFakeGateway();
  fake.program('OpenSession', 'sess-2');
  setGateway(fake);

  await openSession('unknown-conn');
  const list = get(sessions);
  assert(
    list[0].connectionName === 'Session' && list[0].protocol === 'ssh',
    'openSession falls back to connectionName "Session" and protocol "ssh" when the connection is unknown',
  ); // api.ts:343-344
}

// --- closeSession ------------------------------------------------------

// api.ts:358-372 closeSession: removes the tab from `sessions` before
// awaiting the RPC (optimistic removal happens synchronously prior to await).
{
  reset();
  sessions.set([
    { sessionId: 's1', connectionId: 'c1', connectionName: 'A', state: 'ready', errorMessage: '' },
  ]);
  const fake = createFakeGateway();
  let removedBeforeRpc = false;
  fake.program('CloseSession', () => {
    // At the moment the RPC fires, the tab must already be gone from the store.
    removedBeforeRpc = get(sessions).length === 0;
    return undefined;
  });
  setGateway(fake);

  await closeSession('s1');
  assert(removedBeforeRpc, 'closeSession removes the tab from sessions before awaiting the RPC'); // api.ts:362-364
  assert(get(sessions).length === 0, 'session tab is gone after closeSession');
}

// closeSession: a thrown Error('session not found') is swallowed, no lastError set.
{
  reset();
  sessions.set([
    { sessionId: 's1', connectionId: 'c1', connectionName: 'A', state: 'ready', errorMessage: '' },
  ]);
  const fake = createFakeGateway();
  fake.program('CloseSession', () => {
    throw new Error('session not found');
  });
  setGateway(fake);

  await closeSession('s1');
  assert(get(lastError) === null, 'closeSession swallows "session not found" errors without setting lastError'); // api.ts:366-369
}

// closeSession: any other error sets lastError.
{
  reset();
  sessions.set([
    { sessionId: 's1', connectionId: 'c1', connectionName: 'A', state: 'ready', errorMessage: '' },
  ]);
  const fake = createFakeGateway();
  fake.program('CloseSession', () => {
    throw new Error('boom');
  });
  setGateway(fake);

  await closeSession('s1');
  const err = get(lastError);
  assert(err !== null && err.message === 'Close session: boom', 'closeSession sets lastError for non-"not found" errors'); // api.ts:370, api.ts:106-111
}

// --- closeActiveSession -------------------------------------------------

// api.ts:424-434 closeActiveSession with one session -> activeSessionId becomes ''
{
  reset();
  sessions.set([
    { sessionId: 's1', connectionId: 'c1', connectionName: 'A', state: 'ready', errorMessage: '' },
  ]);
  activeSessionId.set('s1');
  const fake = createFakeGateway();
  fake.program('CloseSession', undefined);
  setGateway(fake);

  await closeActiveSession();
  assert(get(activeSessionId) === '', 'closeActiveSession with one session leaves activeSessionId as empty string'); // api.ts:431-432
}

// closeActiveSession with several sessions -> becomes the last remaining one
// (list[list.length - 1], not the previous tab).
{
  reset();
  sessions.set([
    { sessionId: 's1', connectionId: 'c1', connectionName: 'A', state: 'ready', errorMessage: '' },
    { sessionId: 's2', connectionId: 'c2', connectionName: 'B', state: 'ready', errorMessage: '' },
    { sessionId: 's3', connectionId: 'c3', connectionName: 'C', state: 'ready', errorMessage: '' },
  ]);
  activeSessionId.set('s2');
  const fake = createFakeGateway();
  fake.program('CloseSession', undefined);
  setGateway(fake);

  await closeActiveSession();
  const remaining = get(sessions);
  assert(remaining.length === 2 && remaining[0].sessionId === 's1' && remaining[1].sessionId === 's3', 'closing s2 leaves s1 and s3');
  assert(get(activeSessionId) === 's3', 'closeActiveSession picks the last item of the remaining list, not the previously-adjacent tab'); // api.ts:429-430
}

// --- createSessionFromSelection -----------------------------------------

// api.ts:399-405: uses selectedConnectionId if set.
{
  reset();
  connections.set([
    { id: 'c1', folderId: '', name: 'Conn1', host: 'h', port: 22, order: 0 },
    { id: 'c2', folderId: '', name: 'Conn2', host: 'h', port: 22, order: 1 },
  ]);
  selectedConnectionId.set('c2');
  const fake = createFakeGateway();
  fake.program('OpenSession', 'sess-x');
  setGateway(fake);

  await createSessionFromSelection();
  const list = get(sessions);
  assert(list.length === 1 && list[0].connectionId === 'c2', 'createSessionFromSelection uses selectedConnectionId when set'); // api.ts:402
}

// createSessionFromSelection: falls back to first connection when nothing selected.
{
  reset();
  connections.set([
    { id: 'c1', folderId: '', name: 'Conn1', host: 'h', port: 22, order: 0 },
    { id: 'c2', folderId: '', name: 'Conn2', host: 'h', port: 22, order: 1 },
  ]);
  selectedConnectionId.set('');
  const fake = createFakeGateway();
  fake.program('OpenSession', 'sess-y');
  setGateway(fake);

  await createSessionFromSelection();
  const list = get(sessions);
  assert(list.length === 1 && list[0].connectionId === 'c1', 'createSessionFromSelection falls back to the first connection'); // api.ts:402
}

// createSessionFromSelection: zero connections -> no-op (no session opened).
{
  reset();
  connections.set([]);
  selectedConnectionId.set('');
  const fake = createFakeGateway();
  fake.program('OpenSession', 'sess-z');
  setGateway(fake);

  await createSessionFromSelection();
  assert(get(sessions).length === 0, 'createSessionFromSelection with no connections is a no-op'); // api.ts:403
}

// --- focusNextSessionTab / focusPrevSessionTab --------------------------

// api.ts:407-422 cycleSession: wraps around in both directions.
{
  reset();
  sessions.set([
    { sessionId: 's1', connectionId: 'c1', connectionName: 'A', state: 'ready', errorMessage: '' },
    { sessionId: 's2', connectionId: 'c2', connectionName: 'B', state: 'ready', errorMessage: '' },
    { sessionId: 's3', connectionId: 'c3', connectionName: 'C', state: 'ready', errorMessage: '' },
  ]);

  activeSessionId.set('s3');
  focusNextSessionTab();
  assert(get(activeSessionId) === 's1', 'focusNextSessionTab wraps from the last tab back to the first'); // api.ts:412

  activeSessionId.set('s1');
  focusPrevSessionTab();
  assert(get(activeSessionId) === 's3', 'focusPrevSessionTab wraps from the first tab back to the last'); // api.ts:412

  activeSessionId.set('s1');
  focusNextSessionTab();
  assert(get(activeSessionId) === 's2', 'focusNextSessionTab moves forward one tab'); // api.ts:412
}

// focusNextSessionTab with zero sessions is a no-op.
{
  reset();
  activeSessionId.set('');
  focusNextSessionTab();
  assert(get(activeSessionId) === '', 'focusNextSessionTab with no sessions leaves activeSessionId unchanged'); // api.ts:409
}

console.log('sessions.char.test passed');
