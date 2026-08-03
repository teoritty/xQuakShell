import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import { fetchSSHConfigDefaultPath, previewSSHConfig, importSSHConfig } from './sshConfig';
import { lastError } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

const previewPayload = {
  path: '/home/u/.ssh/config',
  hosts: [
    { alias: 'web', hostName: 'web.example.com', port: 2222, user: 'deploy', keyCount: 1, jumpAliases: ['bastion'], duplicate: false },
  ],
  keyFileCount: 1,
  notices: [{ kind: 'matchBlockSkipped', target: '' }],
};

const resultPayload = {
  connections: [{ id: 'c1', name: 'web' }],
  importedKeys: 1,
  failedKeys: 0,
  skippedAliases: [],
};

async function run() {
  let fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);

  fake.program('GetSSHConfigDefaultPath', '/home/u/.ssh/config');
  const detected = await fetchSSHConfigDefaultPath();
  assert(detected === '/home/u/.ssh/config', 'fetchSSHConfigDefaultPath returns the detected path');

  fake.program('PreviewSSHConfig', previewPayload);
  const preview = await previewSSHConfig('/home/u/.ssh/config');
  assert(preview.hosts.length === 1 && preview.hosts[0].alias === 'web', 'previewSSHConfig returns hosts');
  assert(preview.hosts[0].jumpAliases[0] === 'bastion', 'previewSSHConfig keeps jump aliases');
  assert(preview.notices.length === 1, 'previewSSHConfig keeps notices');
  let call = fake.calls.find((c) => c.method === 'PreviewSSHConfig');
  assert(!!call && call.args[0] === '/home/u/.ssh/config', 'PreviewSSHConfig called with the path');

  fake.program('ImportSSHConfig', resultPayload);
  const result = await importSSHConfig('/home/u/.ssh/config', ['web'], 'folder-1', true);
  assert(result.connections.length === 1, 'importSSHConfig returns connections');
  assert(result.importedKeys === 1, 'importSSHConfig returns the key count');
  call = fake.calls.find((c) => c.method === 'ImportSSHConfig');
  assert(!!call && call.args[0] === '/home/u/.ssh/config', 'ImportSSHConfig called with the path');
  assert(!!call && Array.isArray(call.args[1]) && (call.args[1] as string[])[0] === 'web', 'ImportSSHConfig called with aliases');
  assert(!!call && call.args[2] === 'folder-1', 'ImportSSHConfig called with the folder id');
  assert(!!call && call.args[3] === true, 'ImportSSHConfig called with the key flag');

  // The frontend must never hand the backend file contents or a key path;
  // only the config path, aliases, folder and flag are on the wire.
  assert(!!call && call.args.length === 4, 'ImportSSHConfig takes exactly the four documented arguments');

  assert(get(lastError) === null, 'no error reported for successful calls');

  // failure and missing-result behavior
  fake = createFakeGateway();
  setGateway(fake);

  fake.program('GetSSHConfigDefaultPath', () => { throw new Error('boom'); });
  lastError.set(null);
  const detectedFail = await fetchSSHConfigDefaultPath();
  assert(detectedFail === '', 'fetchSSHConfigDefaultPath falls back to empty string');
  assert(get(lastError) !== null, 'fetchSSHConfigDefaultPath failure reports error');

  fake.program('PreviewSSHConfig', () => { throw new Error('boom'); });
  lastError.set(null);
  const previewFail = await previewSSHConfig('/config');
  assert(previewFail.hosts.length === 0 && previewFail.notices.length === 0, 'previewSSHConfig falls back to an empty preview');
  assert(get(lastError) !== null, 'previewSSHConfig failure reports error');

  fake.program('PreviewSSHConfig', undefined);
  lastError.set(null);
  const previewMissing = await previewSSHConfig('/config');
  assert(previewMissing.hosts.length === 0, 'previewSSHConfig tolerates a missing result');
  assert(get(lastError) === null, 'previewSSHConfig missing result is not an error');

  fake.program('PreviewSSHConfig', { path: '/config' });
  lastError.set(null);
  const previewPartial = await previewSSHConfig('/config');
  assert(Array.isArray(previewPartial.hosts) && Array.isArray(previewPartial.notices), 'previewSSHConfig normalizes missing lists to arrays');

  fake.program('ImportSSHConfig', () => { throw new Error('boom'); });
  lastError.set(null);
  const importFail = await importSSHConfig('/config', ['web'], '', false);
  assert(importFail.connections.length === 0 && importFail.importedKeys === 0, 'importSSHConfig falls back to an empty result');
  assert(get(lastError) !== null, 'importSSHConfig failure reports error');

  fake.program('ImportSSHConfig', undefined);
  lastError.set(null);
  const importMissing = await importSSHConfig('/config', ['web'], '', false);
  assert(Array.isArray(importMissing.skippedAliases), 'importSSHConfig tolerates a missing result');

  console.log('sshConfig api tests passed');
}

run().catch((e) => { console.error(e); process.exit(1); });
