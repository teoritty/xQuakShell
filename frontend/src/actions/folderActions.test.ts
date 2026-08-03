import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  refreshFolders,
  saveFolder,
  createNewFolderInFolder,
  deleteFolder,
  moveFolder,
  moveFolders,
  reorderFolders,
} from './folderActions';
import {
  folders, connections,
  selectedFolderId,
  lastError,
  type Folder,
} from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

function reset() {
  folders.set([]);
  connections.set([]);
  selectedFolderId.set('');
  lastError.set(null);
}

async function run() {
  // --- refreshFolders ------------------------------------------------------

  {
    reset();
    const fake = createFakeGateway();
    fake.program('GetFolders', [{ id: 'fa', name: 'A', parentId: '', order: 0 }] as Folder[]);
    setGateway(fake);

    await refreshFolders();
    assert(get(folders).length === 1 && get(folders)[0].id === 'fa', 'refreshFolders replaces folders store with GetFolders result');
  }

  {
    reset();
    const fake = createFakeGateway();
    fake.program('GetFolders', null);
    setGateway(fake);

    await refreshFolders();
    assert(Array.isArray(get(folders)) && get(folders).length === 0, 'refreshFolders falls back to [] when GetFolders returns null');
  }

  // Missing gateway: guarded before any store mutation.
  {
    reset();
    folders.set([{ id: 'f1', name: 'F', parentId: '', order: 0 }]);
    setGateway(null);

    await refreshFolders();
    assert(get(folders).length === 1, 'refreshFolders does not touch folders when gateway is missing');
    assert(get(lastError) === null, 'refreshFolders does not set lastError when gateway is missing');
  }

  // --- saveFolder ------------------------------------------------------------

  {
    reset();
    const fake = createFakeGateway();
    fake.program('SaveFolder', { id: 'fnew', name: 'New', parentId: '', order: 0 });
    fake.program('GetFolders', [{ id: 'fnew', name: 'New', parentId: '', order: 0 }] as Folder[]);
    setGateway(fake);

    const saved = await saveFolder({ name: 'New', parentId: '' });
    assert(saved?.id === 'fnew', 'saveFolder returns the object returned by SaveFolder');
    assert(get(folders).length === 1 && get(folders)[0].id === 'fnew', 'saveFolder triggers a folders refresh after saving');
    const methods = fake.calls.map(c => c.method);
    assert(methods[0] === 'SaveFolder' && methods[1] === 'GetFolders', 'saveFolder calls SaveFolder before GetFolders');
  }

  // Missing gateway: saveFolder has no explicit guard, but putFolder (via
  // callBackend) returns null on a missing gateway, so `if (saved)` skips
  // the refresh — no store mutation, no RPCs at all.
  {
    reset();
    setGateway(null);

    const saved = await saveFolder({ name: 'New', parentId: '' });
    assert(saved === null, 'saveFolder returns null when gateway is missing');
    assert(get(folders).length === 0, 'saveFolder does not touch folders when gateway is missing');
    assert(get(lastError) === null, 'saveFolder does not set lastError when gateway is missing');
  }

  // --- createNewFolderInFolder ----------------------------------------------

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
    assert(payload.name === 'New folder', 'createNewFolderInFolder always names the new folder "New folder"');
    assert(payload.parentId === 'parent-1', 'createNewFolderInFolder nests the new folder under the given parentId');
    assert(get(selectedFolderId) === 'newfolder-1', 'createNewFolderInFolder sets selectedFolderId to the id returned by SaveFolder');
  }

  // createNewFolderInFolder: when saveFolder resolves to null (app absent),
  // selectedFolderId is left untouched.
  {
    reset();
    selectedFolderId.set('previous');
    setGateway(null);
    await createNewFolderInFolder('parent-1');
    assert(get(selectedFolderId) === 'previous', 'createNewFolderInFolder leaves selectedFolderId unchanged when saveFolder returns null');
  }

  // --- deleteFolder ----------------------------------------------------------

  {
    reset();
    const fake = createFakeGateway();
    fake.program('DeleteFolder', undefined);
    fake.program('GetFolders', []);
    fake.program('GetAllConnections', []);
    setGateway(fake);

    await deleteFolder('f1');
    const methods = fake.calls.map(c => c.method);
    assert(methods.includes('GetFolders'), 'deleteFolder refreshes folders');
    assert(methods.includes('GetAllConnections'), 'deleteFolder also refreshes connections');
    assert(methods[0] === 'DeleteFolder' && methods.indexOf('GetFolders') < methods.indexOf('GetAllConnections'), 'deleteFolder calls DeleteFolder, then GetFolders, then GetAllConnections in that order');
  }

  // DeleteFolder RPC throws: both refreshes are skipped.
  {
    reset();
    const fake = createFakeGateway();
    fake.program('DeleteFolder', () => { throw new Error('boom'); });
    setGateway(fake);

    await deleteFolder('f1');
    const methods = fake.calls.map(c => c.method);
    assert(!methods.includes('GetFolders') && !methods.includes('GetAllConnections'), 'deleteFolder skips both refreshes when DeleteFolder throws');
    assert(get(lastError)?.message === 'Delete folder: boom', 'deleteFolder surfaces the DeleteFolder RPC error');
  }

  // Missing gateway: explicit guard reproduced; no RPCs, no refresh.
  {
    reset();
    setGateway(null);
    await deleteFolder('f1');
    assert(get(lastError) === null, 'deleteFolder does not set lastError when gateway is missing');
  }

  // --- moveFolder --------------------------------------------------------

  {
    reset();
    const fake = createFakeGateway();
    fake.program('MoveFolder', undefined);
    fake.program('GetFolders', []);
    setGateway(fake);

    await moveFolder('f1', 'p1');
    const methods = fake.calls.map(c => c.method);
    assert(methods[0] === 'MoveFolder' && methods[1] === 'GetFolders', 'moveFolder calls MoveFolder then refreshes folders');
  }

  {
    reset();
    setGateway(null);
    await moveFolder('f1', 'p1');
    assert(get(lastError) === null, 'moveFolder does not set lastError when gateway is missing');
  }

  // --- moveFolders -----------------------------------------------------

  {
    reset();
    const fake = createFakeGateway();
    fake.program('MoveFolder', undefined);
    fake.program('GetFolders', [{ id: 'f1', name: 'A', parentId: 'p', order: 0 }] as Folder[]);
    setGateway(fake);

    await moveFolders(['f1', 'f2', 'f3'], 'p');
    const methods = fake.calls.map(c => c.method);
    const moveFolderCalls = fake.calls.filter(c => c.method === 'MoveFolder');
    assert(moveFolderCalls.length === 3, 'moveFolders calls MoveFolder once per folder id');
    assert(
      moveFolderCalls[0].args[0] === 'f1' && moveFolderCalls[1].args[0] === 'f2' && moveFolderCalls[2].args[0] === 'f3',
      'moveFolders calls MoveFolder for each id in the given order',
    );
    assert(moveFolderCalls.every(c => c.args[1] === 'p'), 'every MoveFolder call uses the same targetParentId');
    const getFoldersCalls = methods.filter(m => m === 'GetFolders').length;
    assert(getFoldersCalls === 1, 'moveFolders refreshes folders exactly once after the loop, not once per move');
    assert(methods[methods.length - 1] === 'GetFolders', 'the folders refresh happens after all MoveFolder calls');
  }

  // moveFolders: empty id list is a no-op (no RPC calls at all).
  {
    reset();
    const fake = createFakeGateway();
    fake.program('MoveFolder', undefined);
    fake.program('GetFolders', []);
    setGateway(fake);

    await moveFolders([], 'p');
    assert(fake.calls.length === 0, 'moveFolders with an empty id list makes no RPC calls');
  }

  // moveFolders uses the 'Move folders' (plural) error context, distinct
  // from moveFolder's 'Move folder' (singular) context.
  {
    reset();
    const fake = createFakeGateway();
    fake.program('MoveFolder', () => { throw new Error('boom'); });
    setGateway(fake);

    await moveFolders(['f1'], 'p');
    assert(get(lastError)?.message === 'Move folders: boom', 'moveFolders reports errors under the plural "Move folders" context');
  }

  {
    reset();
    const fake = createFakeGateway();
    fake.program('MoveFolder', () => { throw new Error('boom'); });
    setGateway(fake);

    await moveFolder('f1', 'p');
    assert(get(lastError)?.message === 'Move folder: boom', 'moveFolder reports errors under the singular "Move folder" context');
  }

  // Missing gateway: explicit guard reproduced; no RPCs.
  {
    reset();
    setGateway(null);
    await moveFolders(['f1'], 'p');
    assert(get(lastError) === null, 'moveFolders does not set lastError when gateway is missing');
  }

  // --- reorderFolders ------------------------------------------------------

  {
    reset();
    const fake = createFakeGateway();
    fake.program('ReorderFolders', undefined);
    fake.program('GetFolders', []);
    setGateway(fake);

    await reorderFolders(['f1', 'f2'], 'p');
    const methods = fake.calls.map(c => c.method);
    assert(methods[0] === 'ReorderFolders' && methods[1] === 'GetFolders', 'reorderFolders calls ReorderFolders then refreshes folders');
  }

  {
    reset();
    setGateway(null);
    await reorderFolders(['f1'], 'p');
    assert(get(lastError) === null, 'reorderFolders does not set lastError when gateway is missing');
  }

  console.log('folderActions.test passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
