import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  openSession,
  closeSession,
  createSessionFromSelection,
  reconnectSession,
} from './sessionActions';
import { sessions, activeTabId, connections, selectedConnectionId, lastError } from '../stores/appState';
import { tileLayout } from '../stores/tileLayout';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

function reset() {
  sessions.set([]);
  activeTabId.set('');
  connections.set([]);
  selectedConnectionId.set('');
  lastError.set(null);
}

async function run() {
  // --- openSession -------------------------------------------------------

  // openSession adds an optimistic 'connecting' tab immediately, sets
  // activeTabId, and returns the backend session id.
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
    assert(get(activeTabId) === 'sess-1', 'openSession sets activeTabId to the new session id');

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

  // openSession: missing gateway is a silent no-op (no store mutation, no lastError).
  {
    reset();
    connections.set([{ id: 'c1', folderId: '', name: 'MyConn', host: 'h', port: 22, order: 0 }] as any);
    setGateway(null);

    const id = await openSession('c1');
    assert(id === null, 'openSession returns null when gateway is missing');
    assert(get(sessions).length === 0, 'openSession does not touch sessions when gateway is missing');
    assert(get(activeTabId) === '', 'openSession does not touch activeTabId when gateway is missing');
    assert(get(lastError) === null, 'openSession does not set lastError when gateway is missing');
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

  // closeSession: missing gateway is a silent no-op (tab stays, no lastError).
  {
    reset();
    sessions.set([
      { sessionId: 's1', connectionId: 'c1', connectionName: 'A', state: 'ready', errorMessage: '' } as any,
    ]);
    setGateway(null);

    await closeSession('s1');
    const list = get(sessions);
    assert(list.length === 1 && list[0].sessionId === 's1', 'closeSession does not remove the tab when gateway is missing');
    assert(get(lastError) === null, 'closeSession does not set lastError when gateway is missing');
  }

  // --- reconnectSession ------------------------------------------------------

  // The replacement keeps the old tab's tile even when that tile is not the active one: left to
  // reconcile, the old tab's tile would collapse and the new tab would land in the active tile.
  {
    reset();
    sessions.set([
      { sessionId: 'left', connectionId: 'c0', connectionName: 'L', state: 'ready', errorMessage: '' } as any,
      { sessionId: 'old', connectionId: 'c1', connectionName: 'R', protocol: 'telnet', state: 'error', errorMessage: 'x' } as any,
    ]);
    tileLayout.set({
      tiles: [
        { id: 'T1', tabs: ['left'], activeTabId: 'left' },
        { id: 'T2', tabs: ['old'], activeTabId: 'old' },
      ],
      orientation: 'h',
      activeTileId: 'T1',
      dividers: { main: 0.5, cross: 0.5 },
    });
    activeTabId.set('left');
    const fake = createFakeGateway();
    fake.program('OpenSession', 'new');
    fake.program('CloseSession', undefined);
    setGateway(fake);

    const id = await reconnectSession('old', { preserveTerminal: false });
    assert(id === 'new', 'reconnectSession returns the new session id');
    const layout = get(tileLayout);
    assert(layout.tiles.length === 2, 'the old tab\'s tile survives the replacement');
    assert(layout.tiles[1].id === 'T2' && layout.tiles[1].tabs.join() === 'new', 'the new tab sits in the old tab\'s tile');
    assert(layout.tiles[0].tabs.join() === 'left', 'the active tile does not receive the new tab');
    const replaced = get(sessions)[1];
    assert(
      replaced.sessionId === 'new' && replaced.state === 'connecting' && replaced.protocol === 'telnet' && replaced.connectionName === 'R',
      'the replacement starts connecting and keeps the connection\'s identity',
    );
    assert(get(activeTabId) === 'left', 'the focus stays where the user had it');
  }

  // The new session's first state event beat the RPC reply: it is already in the list, appended at
  // the end by the event handler. It must move into place without duplicating, keeping its state.
  {
    reset();
    sessions.set([
      { sessionId: 'old', connectionId: 'c1', connectionName: 'A', state: 'error', errorMessage: '' } as any,
      { sessionId: 'other', connectionId: 'c2', connectionName: 'B', state: 'ready', errorMessage: '' } as any,
    ]);
    activeTabId.set('old');
    const fake = createFakeGateway();
    fake.program('OpenSession', () => {
      sessions.update((l) => [...l, { sessionId: 'new', connectionId: 'c1', connectionName: 'A', state: 'hostkey-required', errorMessage: '' } as any]);
      return 'new';
    });
    fake.program('CloseSession', undefined);
    setGateway(fake);

    await reconnectSession('old', { preserveTerminal: false });
    assert(get(sessions).map((s) => s.sessionId).join() === 'new,other', 'the raced arrival is moved into place, not duplicated');
    assert(get(sessions)[0].state === 'hostkey-required', 'the state the event delivered is kept, not reset to connecting');
    assert(get(activeTabId) === 'new', 'the focus follows the replaced tab');
  }

  // The user closed the tab while the new session was opening: the new session is closed too.
  {
    reset();
    sessions.set([{ sessionId: 'old', connectionId: 'c1', connectionName: 'A', state: 'error', errorMessage: '' } as any]);
    const fake = createFakeGateway();
    fake.program('OpenSession', () => {
      sessions.set([]);
      return 'new';
    });
    fake.program('CloseSession', undefined);
    setGateway(fake);

    const id = await reconnectSession('old', { preserveTerminal: false });
    assert(id === null, 'nothing is replaced when the tab went away meanwhile');
    assert(get(sessions).length === 0, 'the new session does not appear in a closed tab\'s place');
    const closed = fake.calls.filter((c) => c.method === 'CloseSession').map((c) => c.args[0]);
    assert(closed.join() === 'new', 'the orphaned new session is closed on the backend');
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

  console.log('sessionActions.test passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
