import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  changeKeyPassphrase,
  defaultKeyOptions,
  deleteKey,
  deployKey,
  exportKey,
  fetchKeyUsages,
  fetchKeys,
  generateKey,
  importKey,
  planKeyMigration,
  renameKey,
  setKeyPolicy,
} from './keys';
import { lastError } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

const rawKey = {
  id: 'k1',
  comment: 'prod',
  keyType: 'ed25519',
  encrypted: true,
  allowPlugins: false,
  nonExportable: false,
  migrationPending: false,
  policy: 'passphrase',
  cachePolicy: 'duration',
  cacheTtlSeconds: 60,
  fingerprint: 'SHA256:abc',
  publicKey: 'ssh-ed25519 AAAA test',
};

async function run() {
  const fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);

  fake.program('GetKeys', [rawKey]);
  const keys = await fetchKeys();
  assert(keys.length === 1 && keys[0].id === 'k1', 'fetchKeys returns the gateway result');
  assert(keys[0].policy === 'passphrase', 'a known policy survives narrowing');
  assert(keys[0].cachePolicy === 'duration', 'a known cache policy survives narrowing');

  // A private key must not be reachable from a listing under any name. This is the assertion that
  // would catch someone adding a convenience field to the DTO later.
  const forbidden = ['privateKey', 'pem', 'pemData', 'secret', 'blob'];
  for (const field of forbidden) {
    assert(!(field in (keys[0] as Record<string, unknown>)), `listing must not carry ${field}`);
  }

  // An unrecognised policy from an older backend falls back to the pre-policy behaviour rather
  // than reaching the UI as something nothing renders.
  fake.program('GetKeys', [{ ...rawKey, policy: 'something-else', cachePolicy: 'weekly' }]);
  const narrowed = await fetchKeys();
  assert(narrowed[0].policy === 'vault', 'an unknown policy falls back to vault');
  assert(narrowed[0].cachePolicy === 'until-lock', 'an unknown cache policy falls back to until-lock');

  fake.program('GetKeys', null);
  assert((await fetchKeys()).length === 0, 'a null listing becomes an empty array, not a crash');

  fake.program('GetKeyUsages', [{ connectionId: 'c1', connectionName: 'web', username: 'root' }]);
  assert((await fetchKeyUsages('k1')).length === 1, 'fetchKeyUsages returns the gateway result');

  fake.program('GenerateKey', rawKey);
  const generated = await generateKey('ed25519', 0, 'prod', 'pw', defaultKeyOptions);
  assert(generated?.id === 'k1', 'generateKey returns the created key');
  let call = fake.calls.find((c) => c.method === 'GenerateKey');
  assert(!!call && call.args[0] === 'ed25519' && call.args[2] === 'prod', 'GenerateKey passes algorithm and name');
  assert(!!call && call.args[4] === defaultKeyOptions, 'GenerateKey passes the policy options through');

  fake.program('ImportKey', rawKey);
  assert((await importKey('cGVt', 'pw', 'prod', defaultKeyOptions))?.id === 'k1', 'importKey returns the stored key');

  fake.program('RenameKey', undefined);
  await renameKey('k1', 'new name');
  call = fake.calls.find((c) => c.method === 'RenameKey');
  assert(!!call && call.args[1] === 'new name', 'RenameKey passes the new name');

  fake.program('SetKeyPolicy', undefined);
  await setKeyPolicy('k1', defaultKeyOptions);
  assert(!!fake.calls.find((c) => c.method === 'SetKeyPolicy'), 'SetKeyPolicy reaches the backend');

  fake.program('ChangeKeyPassphrase', undefined);
  await changeKeyPassphrase('k1', 'old', 'new');
  call = fake.calls.find((c) => c.method === 'ChangeKeyPassphrase');
  assert(!!call && call.args[1] === 'old' && call.args[2] === 'new', 'ChangeKeyPassphrase passes both passphrases');

  fake.program('ExportKey', 'YmFzZTY0');
  assert((await exportKey('k1', 'master', '', '')) === 'YmFzZTY0', 'exportKey returns the encoded file');
  call = fake.calls.find((c) => c.method === 'ExportKey');
  assert(!!call && call.args[1] === 'master', 'ExportKey sends the master password for re-authentication');

  fake.program('DeployKey', { added: true, alreadyPresent: false, path: '/home/u/.ssh/authorized_keys' });
  assert((await deployKey('s1', 'k1'))?.added === true, 'deployKey returns the result');

  fake.program('PlanKeyMigration', { required: true, keys: [{ id: 'k1', comment: 'prod', keyType: 'rsa' }] });
  const plan = await planKeyMigration('master');
  assert(plan.required && plan.keys.length === 1, 'planKeyMigration reports what the upgrade needs');

  // A refused delete has to reach the caller: it carries the list of connections still using the
  // key, and swallowing it would leave the user with a key that silently will not go away.
  fake.program('DeleteKey', () => {
    throw new Error('SSH identity is still used by connections');
  });
  let threw = false;
  try {
    await deleteKey('k1');
  } catch {
    threw = true;
  }
  assert(threw, 'deleteKey rethrows so the caller can explain the refusal');

  // A failing listing must not take the panel down with it.
  fake.program('GetKeys', () => {
    throw new Error('boom');
  });
  assert((await fetchKeys()).length === 0, 'a failed listing falls back to an empty list');
  assert(get(lastError) !== null, 'a failed listing is reported to the user');

  console.log('keys.test passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
