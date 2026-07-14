import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  unlockVault,
  lockVault,
  refreshFolders,
  refreshAllConnections,
  saveFolder,
  saveConnection,
  deleteFolder,
  createNewConnectionInFolder,
  createNewFolderInFolder,
  moveFolders,
} from '../stores/api';
import {
  folders, connections, sessions, identities, vaultUnlocked,
  selectedConnectionId, detailsConnectionId, selectedFolderId, lastError,
  type Folder, type Connection,
} from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

function reset() {
  folders.set([]);
  connections.set([]);
  sessions.set([]);
  identities.set([]);
  vaultUnlocked.set(false);
  selectedConnectionId.set('');
  detailsConnectionId.set('');
  selectedFolderId.set('');
  lastError.set(null);
}

// --- unlockVault ---------------------------------------------------------

// api.ts:113-125 unlockVault: awaits UnlockVault, sets vaultUnlocked true,
// then fetches platform, folders, connections, identities, protocols and
// applies appearance settings (which calls GetSettings).
{
  reset();
  const fake = createFakeGateway();
  fake.program('UnlockVault', undefined);
  fake.program('GetPlatform', 'linux');
  fake.program('GetFolders', [{ id: 'f1', name: 'F', parentId: '', order: 0 }] as Folder[]);
  fake.program('GetAllConnections', [{ id: 'c1', folderId: 'f1', name: 'C', host: 'h', port: 22, order: 0 }] as Connection[]);
  fake.program('GetIdentities', []);
  fake.program('GetPluginConnectionProtocols', []);
  // GetSettings is deliberately left unprogrammed: it resolves to `undefined`,
  // getSettings() then throws while normalizing hotkeys, is caught internally,
  // and applyAppearanceSettings's `if (!s) return;` short-circuits before
  // touching `document` (which does not exist in this Node test environment).
  // This still exercises the real call path/order up to and including GetSettings.
  setGateway(fake);

  await unlockVault('pw');

  assert(get(vaultUnlocked) === true, 'unlockVault sets vaultUnlocked to true'); // api.ts:117
  assert(get(folders).length === 1 && get(folders)[0].id === 'f1', 'unlockVault populates folders from GetFolders'); // api.ts:120
  assert(get(connections).length === 1 && get(connections)[0].id === 'c1', 'unlockVault populates connections from GetAllConnections'); // api.ts:121
  assert(get(identities).length === 0, 'unlockVault populates identities from GetIdentities'); // api.ts:122

  const methods = fake.calls.map(c => c.method);
  const expectedSubset = ['UnlockVault', 'GetPlatform', 'GetFolders', 'GetAllConnections', 'GetIdentities', 'GetPluginConnectionProtocols', 'GetSettings'];
  for (const m of expectedSubset) {
    assert(methods.includes(m), `unlockVault RPC sequence includes ${m}`); // api.ts:116,118,120-124
  }
  // UnlockVault must be first, GetPlatform second (order matches source body).
  assert(methods[0] === 'UnlockVault', 'UnlockVault is the first RPC call'); // api.ts:116
  assert(methods[1] === 'GetPlatform', 'GetPlatform is the second RPC call'); // api.ts:118
  assert(methods.indexOf('GetFolders') < methods.indexOf('GetAllConnections'), 'GetFolders happens before GetAllConnections'); // api.ts:120-121
  assert(methods.indexOf('GetAllConnections') < methods.indexOf('GetIdentities'), 'GetAllConnections happens before GetIdentities'); // api.ts:121-122
  assert(methods.indexOf('GetIdentities') < methods.indexOf('GetPluginConnectionProtocols'), 'GetIdentities happens before protocol refresh'); // api.ts:122-123
}

// --- lockVault -------------------------------------------------------------

// api.ts:127-140 lockVault: LockVault RPC error is swallowed (via handleError),
// but folders/connections/sessions/identities are still cleared and
// vaultUnlocked is still set to false.
{
  reset();
  folders.set([{ id: 'f1', name: 'F', parentId: '', order: 0 }]);
  connections.set([{ id: 'c1', folderId: 'f1', name: 'C', host: 'h', port: 22, order: 0 }]);
  sessions.set([{ sessionId: 's1', connectionId: 'c1', connectionName: 'C', state: 'ready', errorMessage: '' }]);
  identities.set([{ id: 'i1', comment: '', keyType: 'ed25519' }]);
  vaultUnlocked.set(true);

  const fake = createFakeGateway();
  fake.program('LockVault', () => {
    throw new Error('lock rpc failed');
  });
  setGateway(fake);

  await lockVault();

  assert(get(folders).length === 0, 'lockVault empties folders even when LockVault RPC throws'); // api.ts:136
  assert(get(connections).length === 0, 'lockVault empties connections even when LockVault RPC throws'); // api.ts:137
  assert(get(sessions).length === 0, 'lockVault empties sessions even when LockVault RPC throws'); // api.ts:138
  assert(get(identities).length === 0, 'lockVault empties identities even when LockVault RPC throws'); // api.ts:139
  assert(get(vaultUnlocked) === false, 'lockVault sets vaultUnlocked to false even when LockVault RPC throws'); // api.ts:135
  const err = get(lastError);
  assert(err !== null && err.message === 'Lock vault: lock rpc failed', 'lockVault surfaces the LockVault RPC error via handleError instead of propagating it'); // api.ts:132-134
}

// --- refreshFolders / refreshAllConnections --------------------------------

// api.ts:142-151 refreshFolders: replaces the folders store with GetFolders result.
{
  reset();
  const fake = createFakeGateway();
  fake.program('GetFolders', [{ id: 'fa', name: 'A', parentId: '', order: 0 }] as Folder[]);
  setGateway(fake);

  await refreshFolders();
  assert(get(folders).length === 1 && get(folders)[0].id === 'fa', 'refreshFolders replaces folders store with GetFolders result'); // api.ts:146-147
}

// refreshFolders: null result from GetFolders becomes [] (not left as null).
{
  reset();
  const fake = createFakeGateway();
  fake.program('GetFolders', null);
  setGateway(fake);

  await refreshFolders();
  assert(Array.isArray(get(folders)) && get(folders).length === 0, 'refreshFolders falls back to [] when GetFolders returns null'); // api.ts:147
}

// api.ts:153-162 refreshAllConnections: replaces connections store with GetAllConnections result.
{
  reset();
  const fake = createFakeGateway();
  fake.program('GetAllConnections', [{ id: 'ca', folderId: '', name: 'A', host: 'h', port: 22, order: 0 }] as Connection[]);
  setGateway(fake);

  await refreshAllConnections();
  assert(get(connections).length === 1 && get(connections)[0].id === 'ca', 'refreshAllConnections replaces connections store with GetAllConnections result'); // api.ts:157-158
}

// --- saveFolder / saveConnection (refresh-after-save) -----------------------

// api.ts:175-186 saveFolder: calls SaveFolder, then refreshes folders via GetFolders,
// and returns the saved folder from SaveFolder (not from the refreshed list).
{
  reset();
  const fake = createFakeGateway();
  fake.program('SaveFolder', { id: 'fnew', name: 'New', parentId: '', order: 0 });
  fake.program('GetFolders', [{ id: 'fnew', name: 'New', parentId: '', order: 0 }] as Folder[]);
  setGateway(fake);

  const saved = await saveFolder({ name: 'New', parentId: '' });
  assert(saved?.id === 'fnew', 'saveFolder returns the object returned by SaveFolder'); // api.ts:179-181
  assert(get(folders).length === 1 && get(folders)[0].id === 'fnew', 'saveFolder triggers a folders refresh (GetFolders) after saving'); // api.ts:180
  const methods = fake.calls.map(c => c.method);
  assert(methods[0] === 'SaveFolder' && methods[1] === 'GetFolders', 'saveFolder calls SaveFolder before GetFolders'); // api.ts:179-180
}

// api.ts:200-211 saveConnection: calls SaveConnection, then refreshes connections
// via GetAllConnections, and returns the saved connection from SaveConnection.
{
  reset();
  const fake = createFakeGateway();
  fake.program('SaveConnection', { id: 'cnew', folderId: '', name: 'New', host: '', port: 22, order: 0 });
  fake.program('GetAllConnections', [{ id: 'cnew', folderId: '', name: 'New', host: '', port: 22, order: 0 }] as Connection[]);
  setGateway(fake);

  const saved = await saveConnection({ name: 'New', folderId: '' });
  assert(saved?.id === 'cnew', 'saveConnection returns the object returned by SaveConnection'); // api.ts:204-206
  assert(get(connections).length === 1 && get(connections)[0].id === 'cnew', 'saveConnection triggers a connections refresh (GetAllConnections) after saving'); // api.ts:205
}

// --- deleteFolder ------------------------------------------------------

// api.ts:188-198 deleteFolder: calls DeleteFolder, then refreshes BOTH folders
// (GetFolders) AND connections (GetAllConnections), since connections may live
// in the deleted folder.
{
  reset();
  const fake = createFakeGateway();
  fake.program('DeleteFolder', undefined);
  fake.program('GetFolders', []);
  fake.program('GetAllConnections', []);
  setGateway(fake);

  await deleteFolder('f1');
  const methods = fake.calls.map(c => c.method);
  assert(methods.includes('GetFolders'), 'deleteFolder refreshes folders'); // api.ts:193
  assert(methods.includes('GetAllConnections'), 'deleteFolder also refreshes connections'); // api.ts:194
  assert(methods[0] === 'DeleteFolder' && methods.indexOf('GetFolders') < methods.indexOf('GetAllConnections'), 'deleteFolder calls DeleteFolder, then GetFolders, then GetAllConnections in that order'); // api.ts:192-194
}

// --- createNewConnectionInFolder ----------------------------------------

// api.ts:213-228 createNewConnectionInFolder: generates a uid, creates a
// connection with a single user whose id is that uid and defaultUserId set
// to the same uid, then sets selectedConnectionId/detailsConnectionId to the
// id returned by SaveConnection.
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
  assert(payload.folderId === 'f1', 'createNewConnectionInFolder places the new connection in the given folder'); // api.ts:219
  assert(Array.isArray(payload.users) && payload.users.length === 1, 'createNewConnectionInFolder creates exactly one user'); // api.ts:220
  const uid = payload.users![0].id;
  assert(payload.defaultUserId === uid, 'defaultUserId matches the generated user id'); // api.ts:220-221
  assert(payload.users![0].authMethod === 'key', 'the generated user defaults to key auth'); // api.ts:220
  assert(get(selectedConnectionId) === 'created-1', 'createNewConnectionInFolder sets selectedConnectionId to the new connection id'); // api.ts:224
  assert(get(detailsConnectionId) === 'created-1', 'createNewConnectionInFolder sets detailsConnectionId to the new connection id'); // api.ts:225
}

// --- createNewFolderInFolder ----------------------------------------------

// api.ts:230-238 createNewFolderInFolder: calls saveFolder with a fixed
// 'New folder' name under the given parentId, then sets selectedFolderId to
// the id SaveFolder returned (not to parentId).
{
  reset();
  const fake = createFakeGateway();
  fake.program('SaveFolder', (payload: unknown) => {
    const p = payload as Folder;
    return { ...p, id: 'newfolder-1' };
  });
  fake.program('GetFolders', []);
  setGateway(fake);

  await createNewFolderInFolder('parent-1');
  const call = fake.calls.find(c => c.method === 'SaveFolder');
  const payload = call?.args[0] as Folder;
  assert(payload.name === 'New folder', 'createNewFolderInFolder always names the new folder "New folder"'); // api.ts:232
  assert(payload.parentId === 'parent-1', 'createNewFolderInFolder nests the new folder under the given parentId'); // api.ts:233
  assert(get(selectedFolderId) === 'newfolder-1', 'createNewFolderInFolder sets selectedFolderId to the id returned by SaveFolder'); // api.ts:235-236
}

// createNewFolderInFolder: when saveFolder resolves to null (app absent),
// selectedFolderId is left untouched.
{
  reset();
  selectedFolderId.set('previous');
  setGateway(null);
  await createNewFolderInFolder('parent-1');
  assert(get(selectedFolderId) === 'previous', 'createNewFolderInFolder leaves selectedFolderId unchanged when saveFolder returns null'); // api.ts:235
}

// --- moveFolders ---------------------------------------------------------

// api.ts:273-284 moveFolders: calls MoveFolder once per id in the given order,
// then performs a single folders refresh at the end (not one per move).
{
  reset();
  const fake = createFakeGateway();
  fake.program('MoveFolder', undefined);
  fake.program('GetFolders', [{ id: 'f1', name: 'A', parentId: 'p', order: 0 }] as Folder[]);
  setGateway(fake);

  await moveFolders(['f1', 'f2', 'f3'], 'p');
  const methods = fake.calls.map(c => c.method);
  const moveFolderCalls = fake.calls.filter(c => c.method === 'MoveFolder');
  assert(moveFolderCalls.length === 3, 'moveFolders calls MoveFolder once per folder id'); // api.ts:277-279
  assert(
    moveFolderCalls[0].args[0] === 'f1' && moveFolderCalls[1].args[0] === 'f2' && moveFolderCalls[2].args[0] === 'f3',
    'moveFolders calls MoveFolder for each id in the given order',
  ); // api.ts:277-279
  assert(moveFolderCalls.every(c => c.args[1] === 'p'), 'every MoveFolder call uses the same targetParentId'); // api.ts:278
  const getFoldersCalls = methods.filter(m => m === 'GetFolders').length;
  assert(getFoldersCalls === 1, 'moveFolders refreshes folders exactly once after the loop, not once per move'); // api.ts:280
  assert(methods[methods.length - 1] === 'GetFolders', 'the folders refresh happens after all MoveFolder calls'); // api.ts:280
}

// moveFolders: empty id list is a no-op (no RPC calls at all).
{
  reset();
  const fake = createFakeGateway();
  fake.program('MoveFolder', undefined);
  fake.program('GetFolders', []);
  setGateway(fake);

  await moveFolders([], 'p');
  assert(fake.calls.length === 0, 'moveFolders with an empty id list makes no RPC calls'); // api.ts:275
}

console.log('library.char.test passed');
