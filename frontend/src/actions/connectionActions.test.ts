import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  refreshAllConnections,
  refreshIdentities,
  saveConnection,
  createNewConnectionInFolder,
  deleteConnection,
  moveConnections,
  reorderConnections,
} from './connectionActions';
import {
  connections, identities,
  selectedConnectionId, detailsConnectionId,
  lastError,
  type Connection,
} from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

function reset() {
  connections.set([]);
  identities.set([]);
  selectedConnectionId.set('');
  detailsConnectionId.set('');
  lastError.set(null);
}

async function run() {
  // --- refreshAllConnections ------------------------------------------

  {
    reset();
    const fake = createFakeGateway();
    fake.program('GetAllConnections', [{ id: 'ca', folderId: '', name: 'A', host: 'h', port: 22, order: 0 }] as Connection[]);
    setGateway(fake);

    await refreshAllConnections();
    assert(get(connections).length === 1 && get(connections)[0].id === 'ca', 'refreshAllConnections replaces connections store with GetAllConnections result');
  }

  {
    reset();
    const fake = createFakeGateway();
    fake.program('GetAllConnections', null);
    setGateway(fake);

    await refreshAllConnections();
    assert(Array.isArray(get(connections)) && get(connections).length === 0, 'refreshAllConnections falls back to [] when GetAllConnections returns null');
  }

  // Missing gateway: guarded before any store mutation.
  {
    reset();
    connections.set([{ id: 'c1', folderId: '', name: 'C', host: 'h', port: 22, order: 0 }]);
    setGateway(null);

    await refreshAllConnections();
    assert(get(connections).length === 1, 'refreshAllConnections does not touch connections when gateway is missing');
    assert(get(lastError) === null, 'refreshAllConnections does not set lastError when gateway is missing');
  }

  // --- refreshIdentities -------------------------------------------------

  {
    reset();
    const fake = createFakeGateway();
    fake.program('GetIdentities', [{ id: 'i1', comment: '', keyType: 'ed25519' }]);
    setGateway(fake);

    await refreshIdentities();
    assert(get(identities).length === 1 && get(identities)[0].id === 'i1', 'refreshIdentities replaces identities store with GetIdentities result');
  }

  {
    reset();
    identities.set([{ id: 'i1', comment: '', keyType: 'ed25519' } as any]);
    setGateway(null);

    await refreshIdentities();
    assert(get(identities).length === 1, 'refreshIdentities does not touch identities when gateway is missing');
    assert(get(lastError) === null, 'refreshIdentities does not set lastError when gateway is missing');
  }

  // --- saveConnection ------------------------------------------------------

  {
    reset();
    const fake = createFakeGateway();
    fake.program('SaveConnection', { id: 'cnew', folderId: '', name: 'New', host: '', port: 22, order: 0 });
    fake.program('GetAllConnections', [{ id: 'cnew', folderId: '', name: 'New', host: '', port: 22, order: 0 }] as Connection[]);
    setGateway(fake);

    const saved = await saveConnection({ name: 'New', folderId: '' });
    assert(saved?.id === 'cnew', 'saveConnection returns the object returned by SaveConnection');
    assert(get(connections).length === 1 && get(connections)[0].id === 'cnew', 'saveConnection triggers a connections refresh after saving');
    const methods = fake.calls.map(c => c.method);
    assert(methods[0] === 'SaveConnection' && methods[1] === 'GetAllConnections', 'saveConnection calls SaveConnection before GetAllConnections');
  }

  // Missing gateway: saveConnection has no explicit guard, but putConnection
  // (via callBackend) returns null on a missing gateway, so `if (saved)`
  // skips the refresh — no store mutation, no RPCs at all.
  {
    reset();
    setGateway(null);

    const saved = await saveConnection({ name: 'New', folderId: '' });
    assert(saved === null, 'saveConnection returns null when gateway is missing');
    assert(get(connections).length === 0, 'saveConnection does not touch connections when gateway is missing');
    assert(get(lastError) === null, 'saveConnection does not set lastError when gateway is missing');
  }

  // --- createNewConnectionInFolder ----------------------------------------

  {
    reset();
    const fake = createFakeGateway();
    fake.program('SaveConnection', (payload: unknown) => {
      const p = payload as Connection;
      return { ...p, id: 'created-1' };
    });
    fake.program('GetAllConnections', []);
    setGateway(fake);

    const saved = await createNewConnectionInFolder('f1');
    assert(saved !== null, 'createNewConnectionInFolder returns the saved connection');
    const call = fake.calls.find(c => c.method === 'SaveConnection');
    const payload = call?.args[0] as Connection;
    assert(payload.folderId === 'f1', 'createNewConnectionInFolder places the new connection in the given folder');
    assert(Array.isArray(payload.users) && payload.users.length === 1, 'createNewConnectionInFolder creates exactly one user');
    const uid = payload.users![0].id;
    assert(typeof uid === 'string' && uid.startsWith('u-'), 'generated user id has the u- prefix');
    assert(payload.defaultUserId === uid, 'defaultUserId matches the generated user id');
    assert(payload.users![0].authMethod === 'key', 'the generated user defaults to key auth');
    assert(get(selectedConnectionId) === 'created-1', 'createNewConnectionInFolder sets selectedConnectionId to the new connection id');
    assert(get(detailsConnectionId) === 'created-1', 'createNewConnectionInFolder sets detailsConnectionId to the new connection id');
  }

  // Missing gateway: relies on saveConnection returning null.
  {
    reset();
    selectedConnectionId.set('previous');
    detailsConnectionId.set('previous');
    setGateway(null);

    const saved = await createNewConnectionInFolder('f1');
    assert(saved === null, 'createNewConnectionInFolder returns null when gateway is missing');
    assert(get(selectedConnectionId) === 'previous', 'createNewConnectionInFolder leaves selectedConnectionId unchanged when gateway is missing');
    assert(get(detailsConnectionId) === 'previous', 'createNewConnectionInFolder leaves detailsConnectionId unchanged when gateway is missing');
  }

  // --- deleteConnection ------------------------------------------------

  {
    reset();
    const fake = createFakeGateway();
    fake.program('DeleteConnection', undefined);
    fake.program('GetAllConnections', []);
    setGateway(fake);

    await deleteConnection('c1');
    const methods = fake.calls.map(c => c.method);
    assert(methods[0] === 'DeleteConnection' && methods[1] === 'GetAllConnections', 'deleteConnection calls DeleteConnection then GetAllConnections');
  }

  // DeleteConnection RPC throws: refresh is skipped (error already reported
  // by deleteConnectionById's callBackendVoid).
  {
    reset();
    const fake = createFakeGateway();
    fake.program('DeleteConnection', () => { throw new Error('boom'); });
    fake.program('GetAllConnections', []);
    setGateway(fake);

    await deleteConnection('c1');
    const methods = fake.calls.map(c => c.method);
    assert(!methods.includes('GetAllConnections'), 'deleteConnection skips the refresh when DeleteConnection throws');
    assert(get(lastError)?.message === 'Delete connection: boom', 'deleteConnection surfaces the DeleteConnection RPC error');
  }

  // Missing gateway: explicit guard reproduced; no RPCs, no refresh.
  {
    reset();
    setGateway(null);

    await deleteConnection('c1');
    assert(get(lastError) === null, 'deleteConnection does not set lastError when gateway is missing');
  }

  // --- moveConnections ---------------------------------------------------

  {
    reset();
    const fake = createFakeGateway();
    fake.program('MoveConnections', undefined);
    fake.program('GetAllConnections', []);
    setGateway(fake);

    await moveConnections(['c1', 'c2'], 'f2');
    const methods = fake.calls.map(c => c.method);
    assert(methods[0] === 'MoveConnections' && methods[1] === 'GetAllConnections', 'moveConnections calls MoveConnections then refreshes connections');
    const call = fake.calls.find(c => c.method === 'MoveConnections');
    assert(JSON.stringify(call?.args[0]) === JSON.stringify(['c1', 'c2']) && call?.args[1] === 'f2', 'moveConnections passes ids and target folder through');
  }

  {
    reset();
    setGateway(null);
    await moveConnections(['c1'], 'f2');
    assert(get(lastError) === null, 'moveConnections does not set lastError when gateway is missing');
  }

  // --- reorderConnections --------------------------------------------------

  {
    reset();
    const fake = createFakeGateway();
    fake.program('ReorderConnections', undefined);
    fake.program('GetAllConnections', []);
    setGateway(fake);

    await reorderConnections(['c1', 'c2'], 'f1');
    const methods = fake.calls.map(c => c.method);
    assert(methods[0] === 'ReorderConnections' && methods[1] === 'GetAllConnections', 'reorderConnections calls ReorderConnections then refreshes connections');
  }

  {
    reset();
    setGateway(null);
    await reorderConnections(['c1'], 'f1');
    assert(get(lastError) === null, 'reorderConnections does not set lastError when gateway is missing');
  }

  console.log('connectionActions.test passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
