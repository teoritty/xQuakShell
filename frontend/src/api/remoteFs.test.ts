import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  listPath,
  uploadFile,
  downloadFile,
  cancelTransfer,
  removePath,
  mkdirPath,
  createFilePath,
  copyLocalPath,
  renamePath,
  chmodPath,
  chownPath,
  chmodPathRecursive,
  chownPathRecursive,
} from './remoteFs';
import { lastError } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

async function run() {
  // representative pass-through of args
  let fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);
  await chmodPath('s', '/p', 0o644);
  let call = fake.calls.find((c) => c.method === 'Chmod');
  assert(!!call && call.args[0] === 's' && call.args[1] === '/p' && call.args[2] === 0o644, 'Chmod called with args');

  await chownPath('s', '/p', 1000, 1001);
  call = fake.calls.find((c) => c.method === 'Chown');
  assert(!!call && call.args[2] === 1000 && call.args[3] === 1001, 'Chown called with args');

  await chmodPathRecursive('s', '/p', 0o755, 'both');
  call = fake.calls.find((c) => c.method === 'ChmodRecursive');
  assert(!!call && call.args[3] === 'both', 'ChmodRecursive called with applyTo');

  await chownPathRecursive('s', '/p', 1000, 1001, 'files');
  call = fake.calls.find((c) => c.method === 'ChownRecursive');
  assert(!!call && call.args[4] === 'files', 'ChownRecursive called with applyTo');

  await mkdirPath('s', '/p', 'newdir');
  call = fake.calls.find((c) => c.method === 'MkdirPath');
  assert(!!call && call.args[2] === 'newdir', 'MkdirPath called with args');

  await createFilePath('s', '/p', 'newfile');
  call = fake.calls.find((c) => c.method === 'CreateFilePath');
  assert(!!call && call.args[2] === 'newfile', 'CreateFilePath called with args');

  await copyLocalPath('/src', '/dest');
  call = fake.calls.find((c) => c.method === 'CopyLocalPath');
  assert(!!call && call.args[0] === '/src' && call.args[1] === '/dest', 'CopyLocalPath called with args');

  await renamePath('s', '/old', '/new');
  call = fake.calls.find((c) => c.method === 'RenamePath');
  assert(!!call && call.args[1] === '/old' && call.args[2] === '/new', 'RenamePath called with args');

  await removePath('s', '/p');
  call = fake.calls.find((c) => c.method === 'RemovePath');
  assert(!!call, 'RemovePath called');

  cancelTransfer('t1');
  call = fake.calls.find((c) => c.method === 'CancelTransfer');
  assert(!!call && call.args[0] === 't1', 'CancelTransfer called with id');

  assert(get(lastError) === null, 'no error reported for successful calls');

  // cancel-message errors are swallowed silently for upload/download
  fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);
  fake.program('Upload', () => { throw new Error('transfer cancelled'); });
  await uploadFile('s', 'l', 'r');
  assert(get(lastError) === null, 'cancel error on upload is not surfaced');

  lastError.set(null);
  fake.program('Download', () => { throw new Error('Cancelled by user'); });
  await downloadFile('s', 'r', 'l');
  assert(get(lastError) === null, 'cancel error on download is not surfaced (case-insensitive)');

  // non-cancel errors ARE surfaced for upload/download
  lastError.set(null);
  fake.program('Upload', () => { throw new Error('disk full'); });
  await uploadFile('s', 'l', 'r');
  assert(get(lastError)?.message === 'Upload file: disk full', 'non-cancel upload error is surfaced');

  // missing-gateway fallbacks
  setGateway(null);
  lastError.set(null);
  const nodes = await listPath('s', '/p');
  assert(Array.isArray(nodes) && nodes.length === 0, 'listPath falls back to [] when gateway missing');

  const chmodResult = await chmodPath('s', '/p', 0o644);
  assert(chmodResult === undefined, 'chmodPath falls back to undefined when gateway missing');

  const uploadResult = await uploadFile('s', 'l', 'r');
  assert(uploadResult === undefined, 'uploadFile falls back to undefined when gateway missing');

  assert(get(lastError) === null, 'no error reported when gateway missing');

  console.log('remoteFs.test passed');
}

run();
