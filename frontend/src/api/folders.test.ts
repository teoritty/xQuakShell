import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  fetchFolders,
  putFolder,
  deleteFolderById,
  moveFolderTo,
  reorderFoldersIn,
} from './folders';
import { lastError } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

async function run() {
  let fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);

  const folder = { id: 'f1', name: 'Folder', parentId: '' };
  fake.program('GetFolders', [folder]);
  let result = await fetchFolders();
  assert(result.length === 1 && result[0].id === 'f1', 'fetchFolders returns gateway result');

  fake.program('SaveFolder', folder);
  let saved = await putFolder({ name: 'Folder' });
  assert(saved?.id === 'f1', 'putFolder returns saved folder');
  let call = fake.calls.find((c) => c.method === 'SaveFolder');
  assert(!!call && (call.args[0] as any).name === 'Folder', 'SaveFolder called with payload');

  await deleteFolderById('f1');
  call = fake.calls.find((c) => c.method === 'DeleteFolder');
  assert(!!call && call.args[0] === 'f1', 'DeleteFolder called with id');

  await moveFolderTo('f1', 'f2');
  call = fake.calls.find((c) => c.method === 'MoveFolder');
  assert(!!call && call.args[0] === 'f1' && call.args[1] === 'f2', 'MoveFolder called with args');

  await reorderFoldersIn(['f1', 'f2'], 'p1');
  call = fake.calls.find((c) => c.method === 'ReorderFolders');
  assert(!!call && (call.args[0] as string[]).length === 2 && call.args[1] === 'p1', 'ReorderFolders called with args');

  assert(get(lastError) === null, 'no error reported for successful calls');

  // fallback behavior on failure
  fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);
  fake.program('GetFolders', () => { throw new Error('boom'); });
  result = await fetchFolders();
  assert(Array.isArray(result) && result.length === 0, 'fetchFolders falls back to []');
  assert(get(lastError) !== null, 'fetchFolders failure reports error');

  fake.program('SaveFolder', () => { throw new Error('boom'); });
  lastError.set(null);
  saved = await putFolder({ name: 'x' });
  assert(saved === null, 'putFolder falls back to null');
  assert(get(lastError) !== null, 'putFolder failure reports error');

  // no gateway
  setGateway(null as any);
  lastError.set(null);
  result = await fetchFolders();
  assert(Array.isArray(result) && result.length === 0, 'fetchFolders with no gateway returns []');
  saved = await putFolder({ name: 'x' });
  assert(saved === null, 'putFolder with no gateway returns null');
  await deleteFolderById('f1'); // should not throw
  await moveFolderTo('f1', 'f2'); // should not throw
  await reorderFoldersIn(['f1'], 'p1'); // should not throw

  console.log('folders.test.ts: all assertions passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
