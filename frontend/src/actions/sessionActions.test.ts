import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  openSession,
  closeSession,
  closeActiveSession,
  createSessionFromSelection,
  focusNextSessionTab,
  focusPrevSessionTab,
} from './sessionActions';
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

async function run() {
  // --- openSession -------------------------------------------------------

  // openSession adds an optimistic 'connecting' tab immediately, sets
  // activeSessionId, and returns the backend session id.
  {
    reset();
    connections.set([{ id: 'c1', folderId: '', name: 'MyConn', host: 'h', port: 22, order: 0 }] as any);
    const fake = createFakeGateway();
    fake.program('OpenSession', 'sess-1');
    setGateway(fake);

    const id = await openSession('c1');
    assert(id === 'sess-1', 'openSession returns the backend session id');
    const list = get(sessions);
    assert(list.length === 1, 'openSession adds exactly one optimistic tab');
    assert(
      list[0].sessionId === 'sess-1' &&
        list[0].connectionId === 'c1' &&
        list[0].connectionName === 'MyConn' &&
        list[0].protocol === 'ssh' &&
        list[0].state === 'connecting' &&
        list[0].errorMessage === '',
      'optimistic tab has state "connecting" and the right shape',
    );
    assert(get(activeSessionId) === 'sess-1', 'openSession sets activeSessionId to the new session id');

    const openCall = fake.calls.find((c) => c.method === 'OpenSession');
    assert(!!openCall && openCall.args[0] === 'c1', 'openSession forwards connectionId to the RPC');

    // Repeat call resolving to the same sessionId does not duplicate the tab.
    await openSession('c1');
    assert(get(sessions).length === 1, 'repeat openSession with same resulting sessionId does not duplicate the tab');
  }

  // openSession: unknown connection falls back to 'Session' / 'ssh'.
  {
    reset();
    const fake = createFakeGateway();
    fake.program('OpenSession', 'sess-2');
    setGateway(fake);

    await openSession('unknown-conn');
    const list = get(sessions);
    assert(
      list[0].connectionName === 'Session' && list[0].protocol === 'ssh',
      'openSession falls back to connectionName "Session" and protocol "ssh" when connection unknown',
    );
  }

  // openSession: RPC failure sets lastError, returns null, no tab added.
  {
    reset();
    const fake = createFakeGateway();
    fake.program('OpenSession', () => { throw new Error('boom'); });
    setGateway(fake);

    const id = await openSession('c1');
    assert(id === null, 'openSession returns null on RPC failure');
    assert(get(sessions).length === 0, 'openSession does not add a tab on RPC failure');
    const err = get(lastError);
    assert(err !== null && err.message === 'Open session: boom', 'openSession reports failure via handleError');
  }

  // --- closeSession --------------------------------------------------------

  // closeSession removes the tab from `sessions` before awaiting the RPC.
  {
    reset();
    sessions.set([
      { sessionId: 's1', connectionId: 'c1', connectionName: 'A', state: 'ready', errorMessage: '' } as any,
    ]);
    const fake = createFakeGateway();
    let removedBeforeRpc = false;
    fake.program('CloseSession', () => {
      removedBeforeRpc = get(sessions).length === 0;
      return undefined;
    });
    setGateway(fake);

    await closeSession('s1');
    assert(removedBeforeRpc, 'closeSession removes the tab from sessions before awaiting the RPC');
    assert(get(sessions).length === 0, 'session tab is gone after closeSession');
  }

  // closeSession: 'session not found' is swallowed by this layer, no lastError.
  {
    reset();
    sessions.set([
      { sessionId: 's1', connectionId: 'c1', connectionName: 'A', state: 'ready', errorMessage: '' } as any,
    ]);
    const fake = createFakeGateway();
    fake.program('CloseSession', () => { throw new Error('Session Not Found'); });
    setGateway(fake);

    await closeSession('s1');
    assert(get(lastError) === null, 'closeSession swallows "session not found" errors (case-insensitive) without setting lastError');
  }

  // closeSession: other errors set lastError.
  {
    reset();
    sessions.set([
      { sessionId: 's1', connectionId: 'c1', connectionName: 'A', state: 'ready', errorMessage: '' } as any,
    ]);
    const fake = createFakeGateway();
    fake.program('CloseSession', () => { throw new Error('boom'); });
    setGateway(fake);

    await closeSession('s1');
    const err = get(lastError);
    assert(err !== null && err.message === 'Close session: boom', 'closeSession sets lastError for non-"not found" errors');
  }

  // --- closeActiveSession ---------------------------------------------------

  {
    reset();
    sessions.set([
      { sessionId: 's1', connectionId: 'c1', connectionName: 'A', state: 'ready', errorMessage: '' } as any,
    ]);
    activeSessionId.set('s1');
    const fake = createFakeGateway();
    fake.program('CloseSession', undefined);
    setGateway(fake);

    await closeActiveSession();
    assert(get(activeSessionId) === '', 'closeActiveSession with one session leaves activeSessionId as empty string');
  }

  {
    reset();
    sessions.set([
      { sessionId: 's1', connectionId: 'c1', connectionName: 'A', state: 'ready', errorMessage: '' } as any,
      { sessionId: 's2', connectionId: 'c2', connectionName: 'B', state: 'ready', errorMessage: '' } as any,
      { sessionId: 's3', connectionId: 'c3', connectionName: 'C', state: 'ready', errorMessage: '' } as any,
    ]);
    activeSessionId.set('s2');
    const fake = createFakeGateway();
    fake.program('CloseSession', undefined);
    setGateway(fake);

    await closeActiveSession();
    const remaining = get(sessions);
    assert(remaining.length === 2 && remaining[0].sessionId === 's1' && remaining[1].sessionId === 's3', 'closing s2 leaves s1 and s3');
    assert(get(activeSessionId) === 's3', 'closeActiveSession picks the last item of the remaining list');
  }

  // --- createSessionFromSelection -------------------------------------------

  {
    reset();
    connections.set([
      { id: 'c1', folderId: '', name: 'Conn1', host: 'h', port: 22, order: 0 } as any,
      { id: 'c2', folderId: '', name: 'Conn2', host: 'h', port: 22, order: 1 } as any,
    ]);
    selectedConnectionId.set('c2');
    const fake = createFakeGateway();
    fake.program('OpenSession', 'sess-x');
    setGateway(fake);

    await createSessionFromSelection();
    const list = get(sessions);
    assert(list.length === 1 && list[0].connectionId === 'c2', 'createSessionFromSelection uses selectedConnectionId when set');
  }

  {
    reset();
    connections.set([
      { id: 'c1', folderId: '', name: 'Conn1', host: 'h', port: 22, order: 0 } as any,
      { id: 'c2', folderId: '', name: 'Conn2', host: 'h', port: 22, order: 1 } as any,
    ]);
    selectedConnectionId.set('');
    const fake = createFakeGateway();
    fake.program('OpenSession', 'sess-y');
    setGateway(fake);

    await createSessionFromSelection();
    const list = get(sessions);
    assert(list.length === 1 && list[0].connectionId === 'c1', 'createSessionFromSelection falls back to the first connection');
  }

  {
    reset();
    connections.set([]);
    selectedConnectionId.set('');
    const fake = createFakeGateway();
    fake.program('OpenSession', 'sess-z');
    setGateway(fake);

    await createSessionFromSelection();
    assert(get(sessions).length === 0, 'createSessionFromSelection with no connections is a no-op');
  }

  // --- focusNextSessionTab / focusPrevSessionTab -----------------------------

  {
    reset();
    sessions.set([
      { sessionId: 's1', connectionId: 'c1', connectionName: 'A', state: 'ready', errorMessage: '' } as any,
      { sessionId: 's2', connectionId: 'c2', connectionName: 'B', state: 'ready', errorMessage: '' } as any,
      { sessionId: 's3', connectionId: 'c3', connectionName: 'C', state: 'ready', errorMessage: '' } as any,
    ]);

    activeSessionId.set('s3');
    focusNextSessionTab();
    assert(get(activeSessionId) === 's1', 'focusNextSessionTab wraps from the last tab back to the first');

    activeSessionId.set('s1');
    focusPrevSessionTab();
    assert(get(activeSessionId) === 's3', 'focusPrevSessionTab wraps from the first tab back to the last');

    activeSessionId.set('s1');
    focusNextSessionTab();
    assert(get(activeSessionId) === 's2', 'focusNextSessionTab moves forward one tab');
  }

  {
    reset();
    activeSessionId.set('');
    focusNextSessionTab();
    assert(get(activeSessionId) === '', 'focusNextSessionTab with no sessions leaves activeSessionId unchanged');
  }

  console.log('sessionActions.test passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
