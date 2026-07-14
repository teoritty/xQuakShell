import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  removeLocalPath,
  mkdirLocalPath,
  renameLocalPath,
  createLocalFile,
  selectLocalFile,
  selectLocalDirectory,
  listLocalPath,
  getPortableDataRoot,
  getUserHomeDir,
  getTempDir,
  openFileWithSystem,
  startFileWatch,
} from './localFs';
import { lastError } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

async function run() {
  let fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);

  // representative pass-through of args
  await mkdirLocalPath('/new/dir');
  let call = fake.calls.find((c) => c.method === 'MkdirLocalPath');
  assert(!!call && call.args[0] === '/new/dir', 'MkdirLocalPath called with args');

  await renameLocalPath('/old', '/new');
  call = fake.calls.find((c) => c.method === 'RenameLocalPath');
  assert(!!call && call.args[0] === '/old' && call.args[1] === '/new', 'RenameLocalPath called with args');

  await openFileWithSystem('/f', 'code');
  call = fake.calls.find((c) => c.method === 'OpenFileWithSystem');
  assert(!!call && call.args[0] === '/f' && call.args[1] === 'code', 'OpenFileWithSystem called with args');

  await openFileWithSystem('/f');
  call = fake.calls.filter((c) => c.method === 'OpenFileWithSystem').pop();
  assert(!!call && call.args[1] === '', 'OpenFileWithSystem defaults editorPath to empty string');

  assert(get(lastError) === null, 'no error reported for successful calls');

  // getPortableDataRoot: two-step capability probe fallback to GetUserHomeDir
  // when GetPortableDataRoot is not a function on the gateway.
  fake = createFakeGateway();
  fake.program('GetUserHomeDir', '/home/user');
  const noProbe = new Proxy(fake, {
    get(target, prop: string) {
      if (prop === 'GetPortableDataRoot') return undefined;
      return (target as unknown as Record<string, unknown>)[prop];
    },
  }) as typeof fake;
  setGateway(noProbe);
  const root = await getPortableDataRoot();
  assert(root === '/home/user', 'getPortableDataRoot falls back to GetUserHomeDir when method absent');
  call = fake.calls.find((c) => c.method === 'GetUserHomeDir');
  assert(!!call, 'GetUserHomeDir invoked as fallback');

  // when GetPortableDataRoot IS present, it takes precedence
  fake = createFakeGateway();
  fake.program('GetPortableDataRoot', '/portable');
  fake.program('GetUserHomeDir', '/home/user');
  setGateway(fake);
  const root2 = await getPortableDataRoot();
  assert(root2 === '/portable', 'getPortableDataRoot prefers GetPortableDataRoot when present');
  assert(!fake.calls.some((c) => c.method === 'GetUserHomeDir'), 'GetUserHomeDir not called when probe succeeds');

  // getPortableDataRoot / getUserHomeDir / getTempDir swallow errors silently
  // (no showError call), matching the original stores/api.ts bodies which
  // return '' from a bare catch without calling handleError.
  fake = createFakeGateway();
  fake.program('GetPortableDataRoot', () => { throw new Error('boom'); });
  fake.program('GetUserHomeDir', () => { throw new Error('boom2'); });
  fake.program('GetTempDir', () => { throw new Error('boom3'); });
  setGateway(fake);
  lastError.set(null);
  assert((await getPortableDataRoot()) === '', 'getPortableDataRoot falls back to "" on error');
  assert(get(lastError) === null, 'getPortableDataRoot does not surface errors');
  assert((await getUserHomeDir()) === '', 'getUserHomeDir falls back to "" on error');
  assert(get(lastError) === null, 'getUserHomeDir does not surface errors');
  assert((await getTempDir()) === '', 'getTempDir falls back to "" on error');
  assert(get(lastError) === null, 'getTempDir does not surface errors');

  // listLocalPath returns [] on failure
  fake = createFakeGateway();
  fake.program('ListLocalPath', () => { throw new Error('denied'); });
  setGateway(fake);
  lastError.set(null);
  const nodes = await listLocalPath('/p');
  assert(Array.isArray(nodes) && nodes.length === 0, 'listLocalPath falls back to [] on error');
  assert(get(lastError)?.message === 'List local path: denied', 'listLocalPath error IS surfaced via showError');

  // listLocalPath missing-gateway fallback
  setGateway(null);
  lastError.set(null);
  const nodesNoGw = await listLocalPath('/p');
  assert(Array.isArray(nodesNoGw) && nodesNoGw.length === 0, 'listLocalPath falls back to [] when gateway missing');
  assert(get(lastError) === null, 'no error reported when gateway missing');

  // startFileWatch is fire-and-forget: not awaited internally, and a
  // rejected promise from the backend must never surface via showError.
  fake = createFakeGateway();
  fake.program('StartFileWatch', () => { throw new Error('watch failed'); });
  setGateway(fake);
  lastError.set(null);
  startFileWatch('/p'); // must not throw synchronously
  call = fake.calls.find((c) => c.method === 'StartFileWatch');
  assert(!!call && call.args[0] === '/p', 'StartFileWatch invoked with path');
  await new Promise((r) => setTimeout(r, 0));
  assert(get(lastError) === null, 'startFileWatch rejection is not surfaced');

  // missing-gateway fallbacks for void/string functions
  setGateway(null);
  lastError.set(null);
  const removeResult = await removeLocalPath('/p');
  assert(removeResult === undefined, 'removeLocalPath falls back to undefined when gateway missing');
  const selResult = await selectLocalFile();
  assert(selResult === '', 'selectLocalFile falls back to "" when gateway missing');
  const selDirResult = await selectLocalDirectory();
  assert(selDirResult === '', 'selectLocalDirectory falls back to "" when gateway missing');
  const createResult = await createLocalFile('/p');
  assert(createResult === undefined, 'createLocalFile falls back to undefined when gateway missing');
  startFileWatch('/p'); // must be a no-op, not throw
  assert(get(lastError) === null, 'no error reported when gateway missing');

  console.log('localFs.test passed');
}

run();
