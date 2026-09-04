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

  // pingPlugin: the answer is returned rather than discarded.
  //
  // The backend has always sent the plugin's pong payload back; this wrapper threw it away, which
  // is why a successful ping looked exactly like a button that did nothing.
  fake = createFakeGateway();
  fake.program('PingPlugin', () => ({ pluginId: 'p1', result: { pong: 'ok', version: '1.0.5' } }));
  setGateway(fake);
  const pong = await pingPlugin('p1');
  assert(pong.pong === 'ok' && pong.version === '1.0.5', 'pingPlugin returns the plugin answer');

  // A ping that gets no answer is a finding about that plugin, not an application error, so it is
  // raised to the caller rather than routed to the global error dialog. The dialog is modal and
  // shared with crashes; a plugin failing to answer must not look like one.
  fake = createFakeGateway();
  fake.program('PingPlugin', () => { throw new Error('ping failed'); });
  setGateway(fake);
  lastError.set(null);
  let threw: unknown = null;
  try { await pingPlugin('p1'); } catch (e) { threw = e; }
  assert(threw instanceof Error, 'pingPlugin rethrows so the caller can show the reason on the row');
  assert(get(lastError) === null, 'and does not raise the application error dialog');

  // A backend that answers without a payload must not become "cannot read properties of undefined"
  // in the caller.
  fake = createFakeGateway();
  fake.program('PingPlugin', () => ({ pluginId: 'p1' }));
  setGateway(fake);
  const empty = await pingPlugin('p1');
  assert(
    empty !== null && typeof empty === 'object' && Object.keys(empty).length === 0,
    'a payload-less answer is an empty object, never undefined'
  );

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
  try { await savePluginSettings({ trustedPublisherKeys: [], requireSignedPlugins: false, allowUnsandboxedFallback: false }); } catch (e) { threw = e; }
  assert(threw === null, 'savePluginSettings is a silent no-op when absent');

  fake = createFakeGateway();
  fake.program('SavePluginSettings', () => { throw new Error('save failed'); });
  setGateway(fake);
  lastError.set(null);
  threw = null;
  try { await savePluginSettings({ trustedPublisherKeys: [], requireSignedPlugins: true, allowUnsandboxedFallback: false }); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'save failed', 'savePluginSettings rethrows the original error');
  assert(get(lastError)?.message === 'Save plugin settings: save failed', 'savePluginSettings sets lastError before rethrowing');

  // The master password reaches the backend verbatim, and only when the caller supplies one.
  // Sending a password on every save would train the UI to prompt for it constantly; sending
  // none when the user typed one would make the confirm button silently do nothing.
  fake = createFakeGateway();
  fake.program('SavePluginSettings', { saved: true, reauthRequired: false });
  setGateway(fake);
  const trustSettings = { trustedPublisherKeys: [], requireSignedPlugins: true, allowUnsandboxedFallback: false };
  await savePluginSettings(trustSettings);
  let saveCall = fake.calls.find((c) => c.method === 'SavePluginSettings');
  assert(saveCall?.args[1] === '', 'savePluginSettings sends an empty password when none is given');

  fake = createFakeGateway();
  fake.program('SavePluginSettings', { saved: true, reauthRequired: false });
  setGateway(fake);
  await savePluginSettings(trustSettings, 'master-pw');
  saveCall = fake.calls.find((c) => c.method === 'SavePluginSettings');
  assert(saveCall?.args[1] === 'master-pw', 'savePluginSettings forwards the master password unchanged');

  // reauthRequired is a result, not a throw: the caller re-prompts and retries on it, so
  // turning it into an exception here would break the retry and surface a scary error instead.
  fake = createFakeGateway();
  fake.program('SavePluginSettings', { saved: false, reauthRequired: true });
  setGateway(fake);
  lastError.set(null);
  const refused = await savePluginSettings(trustSettings);
  assert(refused.reauthRequired === true, 'savePluginSettings surfaces reauthRequired to the caller');
  assert(refused.saved === false, 'a refused save does not report itself as saved');
  assert(get(lastError) === null, 'a reauth prompt is not an error and must not set lastError');

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
