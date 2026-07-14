import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  listPlugins,
  pingPlugin,
  setPluginEnabled,
  selectPluginSourceDir,
  selectPluginBundleFile,
  getPluginSettings,
  savePluginSettings,
  generatePluginPublisherKeyPair,
  previewPluginInstall,
  installPluginRpc,
} from './plugins';
import { lastError } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

function withoutMethod<T extends object>(target: T, methodName: string): T {
  return new Proxy(target, {
    get(t, prop: string) {
      if (prop === methodName) return undefined;
      return (t as unknown as Record<string, unknown>)[prop];
    },
  }) as T;
}

async function run() {
  let fake = createFakeGateway();

  // listPlugins: [] when absent
  let noMethod = withoutMethod(fake, 'ListPlugins');
  setGateway(noMethod);
  let plugins = await listPlugins();
  assert(Array.isArray(plugins) && plugins.length === 0, 'listPlugins returns [] when method absent');

  // listPlugins: swallows RPC failure, reports via lastError
  fake = createFakeGateway();
  fake.program('ListPlugins', () => { throw new Error('list failed'); });
  setGateway(fake);
  lastError.set(null);
  plugins = await listPlugins();
  assert(Array.isArray(plugins) && plugins.length === 0, 'listPlugins falls back to [] on RPC failure');
  assert(get(lastError)?.message === 'List plugins: list failed', 'listPlugins reports failure via handleError');

  // pingPlugin: RPC failure fully swallowed (no rethrow)
  fake = createFakeGateway();
  fake.program('PingPlugin', () => { throw new Error('ping failed'); });
  setGateway(fake);
  lastError.set(null);
  let threw: unknown = null;
  try { await pingPlugin('p1'); } catch (e) { threw = e; }
  assert(threw === null, 'pingPlugin does not rethrow on RPC failure');
  assert(get(lastError)?.message === 'Ping plugin: ping failed', 'pingPlugin reports failure via handleError');

  // setPluginEnabled: forwards args positionally, swallows failure
  fake = createFakeGateway();
  fake.program('SetPluginEnabled', () => { throw new Error('set failed'); });
  setGateway(fake);
  lastError.set(null);
  threw = null;
  try { await setPluginEnabled('p1', true); } catch (e) { threw = e; }
  assert(threw === null, 'setPluginEnabled does not rethrow on RPC failure');
  const setCall = fake.calls.find((c) => c.method === 'SetPluginEnabled');
  assert(!!setCall && setCall.args[0] === 'p1' && setCall.args[1] === true, 'setPluginEnabled forwards pluginId and enabled positionally');

  // selectPluginSourceDir: '' when absent; returns picked path; falls back on failure
  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'SelectPluginSourceDir');
  setGateway(noMethod);
  let dir = await selectPluginSourceDir();
  assert(dir === '', 'selectPluginSourceDir returns "" when absent');

  fake = createFakeGateway();
  fake.program('SelectPluginSourceDir', '/picked/dir');
  setGateway(fake);
  dir = await selectPluginSourceDir();
  assert(dir === '/picked/dir', 'selectPluginSourceDir returns the chosen path');

  fake = createFakeGateway();
  fake.program('SelectPluginSourceDir', () => { throw new Error('dialog failed'); });
  setGateway(fake);
  lastError.set(null);
  dir = await selectPluginSourceDir();
  assert(dir === '', 'selectPluginSourceDir falls back to "" on RPC failure');
  assert(get(lastError)?.message === 'Select plugin folder: dialog failed', 'selectPluginSourceDir reports failure via handleError');

  // selectPluginBundleFile: same shape
  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'SelectPluginBundleFile');
  setGateway(noMethod);
  let file = await selectPluginBundleFile();
  assert(file === '', 'selectPluginBundleFile returns "" when absent');

  fake = createFakeGateway();
  fake.program('SelectPluginBundleFile', '/picked/bundle.zip');
  setGateway(fake);
  file = await selectPluginBundleFile();
  assert(file === '/picked/bundle.zip', 'selectPluginBundleFile returns the chosen path');

  // getPluginSettings: default shape when absent + on failure
  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'GetPluginSettings');
  setGateway(noMethod);
  let settings = await getPluginSettings();
  assert(
    Array.isArray(settings.trustedPublisherKeys) && settings.trustedPublisherKeys.length === 0 && settings.requireSignedPlugins === false,
    'getPluginSettings returns default shape when absent'
  );

  fake = createFakeGateway();
  fake.program('GetPluginSettings', () => { throw new Error('load failed'); });
  setGateway(fake);
  lastError.set(null);
  settings = await getPluginSettings();
  assert(settings.trustedPublisherKeys.length === 0 && settings.requireSignedPlugins === false, 'getPluginSettings falls back to default shape on failure');
  assert(get(lastError)?.message === 'Load plugin settings: load failed', 'getPluginSettings reports failure via handleError');

  // savePluginSettings: no-op when absent; rethrows AND sets lastError on failure
  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'SavePluginSettings');
  setGateway(noMethod);
  threw = null;
  try { await savePluginSettings({ trustedPublisherKeys: [], requireSignedPlugins: false }); } catch (e) { threw = e; }
  assert(threw === null, 'savePluginSettings is a silent no-op when absent');

  fake = createFakeGateway();
  fake.program('SavePluginSettings', () => { throw new Error('save failed'); });
  setGateway(fake);
  lastError.set(null);
  threw = null;
  try { await savePluginSettings({ trustedPublisherKeys: [], requireSignedPlugins: true }); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'save failed', 'savePluginSettings rethrows the original error');
  assert(get(lastError)?.message === 'Save plugin settings: save failed', 'savePluginSettings sets lastError before rethrowing');

  // generatePluginPublisherKeyPair: default empty when absent; pass-through; fallback on failure
  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'GeneratePluginPublisherKeyPair');
  setGateway(noMethod);
  let keys = await generatePluginPublisherKeyPair();
  assert(keys.publicKey === '' && keys.privateKey === '', 'generatePluginPublisherKeyPair returns empty keypair when absent');

  fake = createFakeGateway();
  fake.program('GeneratePluginPublisherKeyPair', { publicKey: 'pub', privateKey: 'priv' });
  setGateway(fake);
  keys = await generatePluginPublisherKeyPair();
  assert(keys.publicKey === 'pub' && keys.privateKey === 'priv', 'generatePluginPublisherKeyPair returns the RPC result on success');

  fake = createFakeGateway();
  fake.program('GeneratePluginPublisherKeyPair', () => { throw new Error('keygen failed'); });
  setGateway(fake);
  lastError.set(null);
  keys = await generatePluginPublisherKeyPair();
  assert(keys.publicKey === '' && keys.privateKey === '', 'generatePluginPublisherKeyPair falls back to empty keypair on failure');
  assert(get(lastError)?.message === 'Generate publisher keys: keygen failed', 'generatePluginPublisherKeyPair reports failure via handleError');

  // previewPluginInstall: throws when absent
  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'PreviewPluginInstall');
  setGateway(noMethod);
  threw = null;
  try { await previewPluginInstall('/dir'); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'Plugin install is unavailable', 'previewPluginInstall throws when absent');

  // installPluginRpc: throws when absent
  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'InstallPlugin');
  setGateway(noMethod);
  threw = null;
  try { await installPluginRpc('/dir'); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'Plugin install is unavailable', 'installPluginRpc throws "Plugin install is unavailable" when absent');

  // installPluginRpc: on success, returns InstallPlugin's result and forwards args positionally;
  // does NOT call GetPluginConnectionProtocols (atomic — no protocol refresh side effect)
  fake = createFakeGateway();
  fake.program('InstallPlugin', { id: 'p1', name: 'N', version: '1.0', description: '', source: 's', state: 'active', requiresSecretAccess: false, signed: true, enabled: true });
  fake.program('GetPluginConnectionProtocols', [{ id: 'ssh', label: 'SSH' }]);
  setGateway(fake);
  const result = await installPluginRpc('/some/dir', true, false, false, false, false);
  assert((result as { id: string }).id === 'p1', 'installPluginRpc returns the object returned by InstallPlugin');
  const installCall = fake.calls.find((c) => c.method === 'InstallPlugin');
  assert(
    !!installCall &&
    installCall.args[0] === '/some/dir' && installCall.args[1] === true && installCall.args[2] === false &&
    installCall.args[3] === false && installCall.args[4] === false && installCall.args[5] === false,
    'installPluginRpc forwards sourceDir and all five grant flags positionally'
  );
  assert(!fake.calls.some((c) => c.method === 'GetPluginConnectionProtocols'), 'installPluginRpc does NOT trigger a protocols reload (atomic, no side effect)');

  // installPluginRpc: RPC failure sets lastError AND rethrows (does not swallow)
  fake = createFakeGateway();
  fake.program('InstallPlugin', () => { throw new Error('install failed'); });
  setGateway(fake);
  lastError.set(null);
  threw = null;
  try { await installPluginRpc('/dir'); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'install failed', 'installPluginRpc rethrows the original error on RPC failure');
  assert(get(lastError)?.message === 'Install plugin: install failed', 'installPluginRpc reports RPC failures via handleError before rethrowing');

  console.log('plugins.test.ts passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
