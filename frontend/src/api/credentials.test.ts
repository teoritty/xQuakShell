import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  importPassword,
  deletePassword,
  fetchIdentities,
  importIdentity,
  importPuTTYPPK,
  importPuTTYRegPreview,
  importPuTTYRegAsConnections,
} from './credentials';
import { lastError } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

async function run() {
  let fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);

  fake.program('ImportPassword', 'secret-id');
  let result = await importPassword('pw', 'label');
  assert(result === 'secret-id', 'importPassword returns id');
  let call = fake.calls.find((c) => c.method === 'ImportPassword');
  assert(!!call && call.args[0] === 'pw' && call.args[1] === 'label', 'ImportPassword called with args');

  fake.program('DeletePassword', undefined);
  await deletePassword('id1');
  call = fake.calls.find((c) => c.method === 'DeletePassword');
  assert(!!call && call.args[0] === 'id1', 'DeletePassword called with id');

  const identity = { id: 'i1', comment: 'me' };
  fake.program('GetIdentities', [identity]);
  const ids = await fetchIdentities();
  assert(ids.length === 1 && (ids[0] as any).id === 'i1', 'fetchIdentities returns gateway result');

  fake.program('ImportIdentity', 'ident-id');
  result = await importIdentity('pem', 'comment');
  assert(result === 'ident-id', 'importIdentity returns id');
  call = fake.calls.find((c) => c.method === 'ImportIdentity');
  assert(!!call && call.args[0] === 'pem' && call.args[1] === 'comment', 'ImportIdentity called with args');

  fake.program('ImportPuTTYPPK', 'ppk-id');
  result = await importPuTTYPPK('ppk', 'pass');
  assert(result === 'ppk-id', 'importPuTTYPPK returns id');
  call = fake.calls.find((c) => c.method === 'ImportPuTTYPPK');
  assert(!!call && call.args[0] === 'ppk' && call.args[1] === 'pass', 'ImportPuTTYPPK called with args');

  const preview = [{ name: 'n', hostName: 'h', port: 22, userName: 'u' }];
  fake.program('ImportPuTTYReg', preview);
  const previews = await importPuTTYRegPreview('reg');
  assert(previews.length === 1 && previews[0].name === 'n', 'importPuTTYRegPreview returns gateway result');

  const conn = { id: 'c1', name: 'Conn' };
  fake.program('ImportPuTTYRegAsConnections', [conn]);
  const conns = await importPuTTYRegAsConnections('reg', 'folder1');
  assert(conns.length === 1 && (conns[0] as any).id === 'c1', 'importPuTTYRegAsConnections returns gateway result');
  call = fake.calls.find((c) => c.method === 'ImportPuTTYRegAsConnections');
  assert(!!call && call.args[0] === 'reg' && call.args[1] === 'folder1', 'ImportPuTTYRegAsConnections called with args');

  assert(get(lastError) === null, 'no error reported for successful calls');

  // fallback behavior on failure
  fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);

  fake.program('ImportPassword', () => { throw new Error('boom'); });
  result = await importPassword('pw', 'label');
  assert(result === '', 'importPassword falls back to empty string');
  assert(get(lastError) !== null, 'importPassword failure reports error');

  fake.program('ImportIdentity', () => { throw new Error('boom'); });
  lastError.set(null);
  result = await importIdentity('pem', 'comment');
  assert(result === '', 'importIdentity falls back to empty string');
  assert(get(lastError) !== null, 'importIdentity failure reports error');

  fake.program('ImportPuTTYPPK', () => { throw new Error('boom'); });
  lastError.set(null);
  result = await importPuTTYPPK('ppk', 'pass');
  assert(result === '', 'importPuTTYPPK falls back to empty string');
  assert(get(lastError) !== null, 'importPuTTYPPK failure reports error');

  fake.program('ImportPuTTYReg', () => { throw new Error('boom'); });
  lastError.set(null);
  const previewsFail = await importPuTTYRegPreview('reg');
  assert(Array.isArray(previewsFail) && previewsFail.length === 0, 'importPuTTYRegPreview falls back to []');
  assert(get(lastError) !== null, 'importPuTTYRegPreview failure reports error');

  fake.program('ImportPuTTYReg', undefined);
  lastError.set(null);
  const previewsMissing = await importPuTTYRegPreview('reg');
  assert(Array.isArray(previewsMissing) && previewsMissing.length === 0, 'importPuTTYRegPreview falls back to [] on missing result');
  assert(get(lastError) === null, 'importPuTTYRegPreview missing result is not an error');

  fake.program('ImportPuTTYRegAsConnections', () => { throw new Error('boom'); });
  lastError.set(null);
  const connsFail = await importPuTTYRegAsConnections('reg', 'folder1');
  assert(Array.isArray(connsFail) && connsFail.length === 0, 'importPuTTYRegAsConnections falls back to []');
  assert(get(lastError) !== null, 'importPuTTYRegAsConnections failure reports error');

  fake.program('GetIdentities', () => { throw new Error('boom'); });
  lastError.set(null);
  const idsFail = await fetchIdentities();
  assert(Array.isArray(idsFail) && idsFail.length === 0, 'fetchIdentities falls back to []');
  assert(get(lastError) !== null, 'fetchIdentities failure reports error');

  fake.program('DeletePassword', () => { throw new Error('boom'); });
  lastError.set(null);
  await deletePassword('id1'); // should not throw
  assert(get(lastError) !== null, 'deletePassword failure reports error');

  // no gateway
  setGateway(null as any);
  lastError.set(null);
  result = await importPassword('pw', 'label');
  assert(result === '', 'importPassword with no gateway returns ""');
  await deletePassword('id1'); // should not throw
  const idsNoGw = await fetchIdentities();
  assert(Array.isArray(idsNoGw) && idsNoGw.length === 0, 'fetchIdentities with no gateway returns []');
  result = await importIdentity('pem', 'comment');
  assert(result === '', 'importIdentity with no gateway returns ""');
  result = await importPuTTYPPK('ppk', 'pass');
  assert(result === '', 'importPuTTYPPK with no gateway returns ""');
  const previewsNoGw = await importPuTTYRegPreview('reg');
  assert(Array.isArray(previewsNoGw) && previewsNoGw.length === 0, 'importPuTTYRegPreview with no gateway returns []');
  const connsNoGw = await importPuTTYRegAsConnections('reg', 'folder1');
  assert(Array.isArray(connsNoGw) && connsNoGw.length === 0, 'importPuTTYRegAsConnections with no gateway returns []');

  console.log('credentials.test.ts: all assertions passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
