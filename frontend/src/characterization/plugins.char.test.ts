import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  refreshConnectionProtocols,
  getPluginConnectionProtocols,
  invalidateProtocolsCache,
  connectionProtocols,
  installPlugin,
  uninstallGitHubPlugin,
  getPluginSettings,
  savePluginSettings,
  getPluginContributions,
  executePluginCommand,
  addGitHubRepository,
  relayPluginViewMessage,
  previewPluginInstall,
  listGitHubRepositories,
  removeGitHubRepository,
  setGitHubRepositoryTrust,
  fetchGitHubPlugins,
  previewGitHubPluginInstall,
  installGitHubPlugin,
  preparePluginViewPanel,
  releasePluginViewPanel,
  listPlugins,
  pingPlugin,
  setPluginEnabled,
  type ConnectionProtocol,
} from '../stores/api';
import { lastError } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

function reset() {
  lastError.set(null);
  invalidateProtocolsCache(); // api.ts:1202-1204
  connectionProtocols.set([]);
}

const PROTO_A: ConnectionProtocol[] = [{ id: 'ssh', label: 'SSH', fields: [] } as unknown as ConnectionProtocol];
const PROTO_B: ConnectionProtocol[] = [{ id: 'ssh', label: 'SSH', fields: [] } as unknown as ConnectionProtocol, { id: 'ftp', label: 'FTP', fields: [] } as unknown as ConnectionProtocol];

// --- getPluginConnectionProtocols / refreshConnectionProtocols / invalidateProtocolsCache ---

// api.ts:1195-1200 getPluginConnectionProtocols is cache-first: a warm cache
// (protocolsCache non-null) short-circuits and never calls refreshConnectionProtocols/RPC.
{
  reset();
  const fake = createFakeGateway();
  fake.program('GetPluginConnectionProtocols', PROTO_A);
  setGateway(fake);

  const first = await getPluginConnectionProtocols();
  assert(first.length === 1 && first[0].id === 'ssh', 'getPluginConnectionProtocols returns list from RPC on cold cache'); // api.ts:1196-1199
  assert(fake.calls.filter(c => c.method === 'GetPluginConnectionProtocols').length === 1, 'first call issues exactly one RPC'); // api.ts:1178-1188

  // Reprogram with a different list; warm cache must still return the OLD list
  // and must NOT issue a second RPC call.
  fake.program('GetPluginConnectionProtocols', PROTO_B);
  const second = await getPluginConnectionProtocols();
  assert(second.length === 1 && second[0].id === 'ssh', 'warm cache returns the previously cached list, not a fresh RPC result'); // api.ts:1196-1198
  assert(fake.calls.filter(c => c.method === 'GetPluginConnectionProtocols').length === 1, 'second call with warm cache issues NO new GetPluginConnectionProtocols RPC'); // api.ts:1196-1198

  // invalidateProtocolsCache() forces the next call to re-fetch.
  invalidateProtocolsCache(); // api.ts:1202-1204
  const third = await getPluginConnectionProtocols();
  assert(third.length === 2 && third[1].id === 'ftp', 'after invalidateProtocolsCache, getPluginConnectionProtocols re-fetches via RPC'); // api.ts:1199,1178-1188
  assert(fake.calls.filter(c => c.method === 'GetPluginConnectionProtocols').length === 2, 'invalidation causes exactly one additional RPC call'); // api.ts:1196-1199
}

// api.ts:1178-1193 refreshConnectionProtocols: when GetPluginConnectionProtocols
// is absent on the gateway (Wails not bound yet), it returns the CURRENT
// connectionProtocols store value and does not touch the cache.
{
  reset();
  connectionProtocols.set(PROTO_A);
  setGateway(null); // getApp() -> null, so app?.GetPluginConnectionProtocols is undefined

  const result = await refreshConnectionProtocols();
  assert(result.length === 1 && result[0].id === 'ssh', 'refreshConnectionProtocols with absent method returns current connectionProtocols store value'); // api.ts:1180-1182

  // Cache was not written: getPluginConnectionProtocols must go through
  // refreshConnectionProtocols again (still absent), returning the same store value.
  const fake = createFakeGateway();
  fake.program('GetPluginConnectionProtocols', PROTO_B);
  setGateway(null);
  const again = await getPluginConnectionProtocols();
  assert(again.length === 1 && again[0].id === 'ssh', 'getPluginConnectionProtocols after an absent-method refresh still has no warm cache and falls through to refresh again'); // api.ts:1196-1199,1180-1182
  void fake; // fake unused while gateway is null; kept for clarity of intent
}

// refreshConnectionProtocols: RPC error is caught via handleError (lastError set)
// and falls back to the current store value without throwing.
{
  reset();
  connectionProtocols.set(PROTO_A);
  const fake = createFakeGateway();
  fake.program('GetPluginConnectionProtocols', () => { throw new Error('boom'); });
  setGateway(fake);

  const result = await refreshConnectionProtocols();
  assert(result.length === 1 && result[0].id === 'ssh', 'refreshConnectionProtocols falls back to current store value on RPC failure'); // api.ts:1189-1192
  const err = get(lastError);
  assert(err !== null && err.message === 'Load connection protocols: boom', 'refreshConnectionProtocols reports RPC failures via handleError'); // api.ts:1190
}

// --- installPlugin --------------------------------------------------------

// api.ts:1304-1325 installPlugin: on success calls InstallPlugin, then
// invalidates the protocol cache and calls refreshConnectionProtocols
// (evidenced by a GetPluginConnectionProtocols RPC AFTER InstallPlugin).
{
  reset();
  const fake = createFakeGateway();
  fake.program('InstallPlugin', { id: 'p1', name: 'Plugin', enabled: true });
  fake.program('GetPluginConnectionProtocols', PROTO_B);
  setGateway(fake);

  const result = await installPlugin('/some/dir', true, false, false, false, false);
  assert((result as { id: string }).id === 'p1', 'installPlugin returns the object returned by InstallPlugin'); // api.ts:1317,1320
  const methods = fake.calls.map(c => c.method);
  assert(methods[0] === 'InstallPlugin', 'installPlugin calls InstallPlugin first'); // api.ts:1317
  assert(methods.includes('GetPluginConnectionProtocols'), 'installPlugin triggers a protocols reload after installing'); // api.ts:1318-1319
  assert(methods.indexOf('InstallPlugin') < methods.indexOf('GetPluginConnectionProtocols'), 'the protocols reload happens after InstallPlugin'); // api.ts:1317-1319
  const installCall = fake.calls.find(c => c.method === 'InstallPlugin');
  assert(
    installCall?.args[0] === '/some/dir' && installCall.args[1] === true && installCall.args[2] === false && installCall.args[3] === false && installCall.args[4] === false && installCall.args[5] === false,
    'installPlugin forwards sourceDir and all five grant flags positionally to InstallPlugin',
  ); // api.ts:1304-1317
}

// installPlugin: when InstallPlugin is absent on the gateway, throws
// 'Plugin install is unavailable' (same message as previewPluginInstall).
{
  reset();
  setGateway(null);
  let threw: unknown = null;
  try {
    await installPlugin('/dir');
  } catch (e) {
    threw = e;
  }
  assert(threw instanceof Error && threw.message === 'Plugin install is unavailable', 'installPlugin throws "Plugin install is unavailable" when InstallPlugin is absent'); // api.ts:1313-1315
}

// installPlugin: RPC failure sets lastError AND rethrows (does not swallow).
{
  reset();
  const fake = createFakeGateway();
  fake.program('InstallPlugin', () => { throw new Error('install failed'); });
  setGateway(fake);
  let threw: unknown = null;
  try {
    await installPlugin('/dir');
  } catch (e) {
    threw = e;
  }
  assert(threw instanceof Error && threw.message === 'install failed', 'installPlugin rethrows the original error on RPC failure'); // api.ts:1321-1324
  const err = get(lastError);
  assert(err !== null && err.message === 'Install plugin: install failed', 'installPlugin reports RPC failures via handleError before rethrowing'); // api.ts:1322
}

// previewPluginInstall: when absent, throws the same 'Plugin install is unavailable'.
{
  reset();
  setGateway(null);
  let threw: unknown = null;
  try {
    await previewPluginInstall('/dir');
  } catch (e) {
    threw = e;
  }
  assert(threw instanceof Error && threw.message === 'Plugin install is unavailable', 'previewPluginInstall throws "Plugin install is unavailable" when absent'); // api.ts:1298-1300
}

// --- uninstallGitHubPlugin -------------------------------------------------

// api.ts:1485-1496 uninstallGitHubPlugin: on success invalidates the cache and
// refreshes protocols (GetPluginConnectionProtocols called after Uninstall).
{
  reset();
  connectionProtocols.set(PROTO_A);
  // Warm the cache first so we can prove invalidation actually forces a refetch.
  const warmFake = createFakeGateway();
  warmFake.program('GetPluginConnectionProtocols', PROTO_A);
  setGateway(warmFake);
  await getPluginConnectionProtocols();

  const fake = createFakeGateway();
  fake.program('UninstallGitHubPlugin', undefined);
  fake.program('GetPluginConnectionProtocols', PROTO_B);
  setGateway(fake);

  await uninstallGitHubPlugin('plugin-1', true);
  const methods = fake.calls.map(c => c.method);
  assert(methods[0] === 'UninstallGitHubPlugin', 'uninstallGitHubPlugin calls UninstallGitHubPlugin first'); // api.ts:1489
  assert(methods.includes('GetPluginConnectionProtocols'), 'uninstallGitHubPlugin refreshes protocols after uninstalling (proves cache invalidation)'); // api.ts:1490-1491
  const call = fake.calls.find(c => c.method === 'UninstallGitHubPlugin');
  assert(call?.args[0] === 'plugin-1' && call.args[1] === true, 'uninstallGitHubPlugin forwards pluginID and removeData'); // api.ts:1489
  assert(get(connectionProtocols).length === 2, 'the refreshed protocols store reflects the post-uninstall RPC result'); // api.ts:1187,1491
}

// uninstallGitHubPlugin: when absent, throws 'GitHub plugin uninstall unavailable'.
{
  reset();
  setGateway(null);
  let threw: unknown = null;
  try {
    await uninstallGitHubPlugin('p1');
  } catch (e) {
    threw = e;
  }
  assert(threw instanceof Error && threw.message === 'GitHub plugin uninstall unavailable', 'uninstallGitHubPlugin throws when UninstallGitHubPlugin is absent'); // api.ts:1487
}

// --- getPluginSettings ------------------------------------------------------

// api.ts:1259-1270 getPluginSettings: when GetPluginSettings is absent, returns
// the exact default shape { trustedPublisherKeys: [], requireSignedPlugins: false }.
{
  reset();
  setGateway(null);
  const settings = await getPluginSettings();
  assert(
    Array.isArray(settings.trustedPublisherKeys) && settings.trustedPublisherKeys.length === 0 && settings.requireSignedPlugins === false,
    'getPluginSettings returns { trustedPublisherKeys: [], requireSignedPlugins: false } when the method is absent',
  ); // api.ts:1261-1263
}

// getPluginSettings: on RPC failure, also falls back to the same default shape
// (via handleError, not thrown).
{
  reset();
  const fake = createFakeGateway();
  fake.program('GetPluginSettings', () => { throw new Error('load failed'); });
  setGateway(fake);
  const settings = await getPluginSettings();
  assert(
    settings.trustedPublisherKeys.length === 0 && settings.requireSignedPlugins === false,
    'getPluginSettings falls back to the default shape on RPC failure too',
  ); // api.ts:1266-1268
  const err = get(lastError);
  assert(err !== null && err.message === 'Load plugin settings: load failed', 'getPluginSettings reports RPC failure via handleError'); // api.ts:1267
}

// --- savePluginSettings ------------------------------------------------------

// api.ts:1272-1281 savePluginSettings: on failure, sets lastError AND rethrows.
{
  reset();
  const fake = createFakeGateway();
  fake.program('SavePluginSettings', () => { throw new Error('save failed'); });
  setGateway(fake);
  let threw: unknown = null;
  try {
    await savePluginSettings({ trustedPublisherKeys: [], requireSignedPlugins: true });
  } catch (e) {
    threw = e;
  }
  assert(threw instanceof Error && threw.message === 'save failed', 'savePluginSettings rethrows the original error'); // api.ts:1279
  const err = get(lastError);
  assert(err !== null && err.message === 'Save plugin settings: save failed', 'savePluginSettings sets lastError via handleError before rethrowing'); // api.ts:1278
}

// savePluginSettings: when the method is absent, it silently no-ops (no throw).
{
  reset();
  setGateway(null);
  let threw: unknown = null;
  try {
    await savePluginSettings({ trustedPublisherKeys: [], requireSignedPlugins: false });
  } catch (e) {
    threw = e;
  }
  assert(threw === null, 'savePluginSettings is a silent no-op when SavePluginSettings is absent'); // api.ts:1273-1274
}

// --- getPluginContributions --------------------------------------------------

// api.ts:1547-1558 getPluginContributions: when the method is absent, returns
// the exact empty-shape default.
{
  reset();
  setGateway(null);
  const contrib = await getPluginContributions();
  assert(
    contrib.commands.length === 0 && contrib.views.length === 0 && contrib.statusBar.length === 0 && contrib.authMethods.length === 0 && contrib.tunnelProviders.length === 0,
    'getPluginContributions returns { commands:[], views:[], statusBar:[], authMethods:[], tunnelProviders:[] } when absent',
  ); // api.ts:1549-1550
}

// --- executePluginCommand -----------------------------------------------------

// api.ts:1560-1580 executePluginCommand: JSON-string result is parsed to an object.
{
  reset();
  const fake = createFakeGateway();
  fake.program('ExecutePluginCommand', '{"a":"b"}');
  setGateway(fake);
  const result = await executePluginCommand('plug1', 'cmd1', { x: 1 });
  assert((result as unknown as { a: string }).a === 'b', 'executePluginCommand JSON.parses a JSON string result into an object'); // api.ts:1572-1574
  const call = fake.calls.find(c => c.method === 'ExecutePluginCommand');
  assert(call?.args[2] === JSON.stringify({ x: 1 }), 'executePluginCommand serializes args via JSON.stringify before the call'); // api.ts:1569
}

// executePluginCommand: non-JSON string result is wrapped as { message: result }.
{
  reset();
  const fake = createFakeGateway();
  fake.program('ExecutePluginCommand', 'hi');
  setGateway(fake);
  const result = await executePluginCommand('plug1', 'cmd1');
  assert(result.message === 'hi', 'executePluginCommand wraps a non-JSON string result as { message: result }'); // api.ts:1573-1577
  const call = fake.calls.find(c => c.method === 'ExecutePluginCommand');
  assert(call?.args[2] === null, 'executePluginCommand passes null as rawArgs when no args are given'); // api.ts:1569
}

// executePluginCommand: falsy/null RPC result becomes {}.
{
  reset();
  const fake = createFakeGateway();
  fake.program('ExecutePluginCommand', null);
  setGateway(fake);
  const result = await executePluginCommand('plug1', 'cmd1');
  assert(Object.keys(result).length === 0, 'executePluginCommand returns {} when the RPC result is null/falsy'); // api.ts:1571
}

// executePluginCommand: non-string, non-null result (already an object) passes through as-is.
{
  reset();
  const fake = createFakeGateway();
  fake.program('ExecutePluginCommand', { already: 'object' });
  setGateway(fake);
  const result = await executePluginCommand('plug1', 'cmd1');
  assert((result as unknown as { already: string }).already === 'object', 'executePluginCommand passes non-string RPC results through unchanged'); // api.ts:1579
}

// executePluginCommand: when the method is absent, throws 'Plugin commands are unavailable'.
{
  reset();
  setGateway(null);
  let threw: unknown = null;
  try {
    await executePluginCommand('p', 'c');
  } catch (e) {
    threw = e;
  }
  assert(threw instanceof Error && threw.message === 'Plugin commands are unavailable', 'executePluginCommand throws when ExecutePluginCommand is absent'); // api.ts:1566-1568
}

// --- addGitHubRepository (rethrow-after-report pattern shared by GitHub fns) ---

// api.ts:1411-1420 addGitHubRepository: on failure, sets lastError AND rethrows.
{
  reset();
  const fake = createFakeGateway();
  fake.program('AddGitHubRepository', () => { throw new Error('add failed'); });
  setGateway(fake);
  let threw: unknown = null;
  try {
    await addGitHubRepository('https://example.com/repo', true);
  } catch (e) {
    threw = e;
  }
  assert(threw instanceof Error && threw.message === 'add failed', 'addGitHubRepository rethrows the original error'); // api.ts:1418
  const err = get(lastError);
  assert(err !== null && err.message === 'Add GitHub repository: add failed', 'addGitHubRepository reports failure via handleError before rethrowing'); // api.ts:1417
}

// addGitHubRepository: success path forwards { url, trusted } as a single object arg.
{
  reset();
  const fake = createFakeGateway();
  fake.program('AddGitHubRepository', undefined);
  setGateway(fake);
  await addGitHubRepository('https://example.com/repo', false);
  const call = fake.calls.find(c => c.method === 'AddGitHubRepository');
  const arg = call?.args[0] as { url: string; trusted: boolean };
  assert(arg.url === 'https://example.com/repo' && arg.trusted === false, 'addGitHubRepository packs url/trusted into a single object arg'); // api.ts:1415
}

// addGitHubRepository: when absent, throws 'GitHub repositories unavailable'.
{
  reset();
  setGateway(null);
  let threw: unknown = null;
  try {
    await addGitHubRepository('https://example.com/repo', true);
  } catch (e) {
    threw = e;
  }
  assert(threw instanceof Error && threw.message === 'GitHub repositories unavailable', 'addGitHubRepository throws when AddGitHubRepository is absent'); // api.ts:1413
}

// The remaining GitHub functions share the identical try/catch(rethrow) shape;
// spot-check each one's rethrow + lastError behavior and its "absent" error text.
{
  reset();
  const fake = createFakeGateway();
  fake.program('RemoveGitHubRepository', () => { throw new Error('remove failed'); });
  setGateway(fake);
  let threw: unknown = null;
  try { await removeGitHubRepository('url'); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'remove failed', 'removeGitHubRepository rethrows on failure'); // api.ts:1429
  assert(get(lastError)?.message === 'Remove GitHub repository: remove failed', 'removeGitHubRepository reports via handleError'); // api.ts:1428
}
{
  reset();
  setGateway(null);
  let threw: unknown = null;
  try { await setGitHubRepositoryTrust('url', true); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'GitHub repositories unavailable', 'setGitHubRepositoryTrust throws when absent'); // api.ts:1435
}
{
  reset();
  const fake = createFakeGateway();
  fake.program('FetchGitHubPlugins', () => { throw new Error('fetch failed'); });
  setGateway(fake);
  let threw: unknown = null;
  try { await fetchGitHubPlugins('url', true); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'fetch failed', 'fetchGitHubPlugins rethrows on failure'); // api.ts:1451
  assert(get(lastError)?.message === 'Fetch GitHub plugins: fetch failed', 'fetchGitHubPlugins reports via handleError'); // api.ts:1450
  const call = fake.calls.find(c => c.method === 'FetchGitHubPlugins');
  const arg = call?.args[0] as { url: string; forceRefresh: boolean };
  assert(arg.url === 'url' && arg.forceRefresh === true, 'fetchGitHubPlugins packs url/forceRefresh into a single object arg'); // api.ts:1448
}
{
  reset();
  setGateway(null);
  let threw: unknown = null;
  try { await previewGitHubPluginInstall('url'); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'GitHub plugin install unavailable', 'previewGitHubPluginInstall throws when absent'); // api.ts:1457
}
{
  reset();
  const fake = createFakeGateway();
  fake.program('InstallGitHubPlugin', () => { throw new Error('install gh failed'); });
  setGateway(fake);
  let threw: unknown = null;
  try { await installGitHubPlugin('url', 'v1', true, false, false, false, false); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'install gh failed', 'installGitHubPlugin rethrows on failure'); // api.ts:1481
  const call = fake.calls.find(c => c.method === 'InstallGitHubPlugin');
  assert(
    call?.args[0] === 'url' && call.args[1] === 'v1' && call.args[2] === true,
    'installGitHubPlugin forwards repoURL, releaseTag and grant flags positionally',
  ); // api.ts:1478
}
{
  reset();
  setGateway(null);
  const list = await listGitHubRepositories();
  assert(Array.isArray(list) && list.length === 0, 'listGitHubRepositories returns [] when absent (does not throw)'); // api.ts:1402
}

// --- relayPluginViewMessage / preparePluginViewPanel / releasePluginViewPanel ---

// api.ts:1590-1600 relayPluginViewMessage: serializes the message object to
// JSON before passing it to RelayPluginViewMessage.
{
  reset();
  const fake = createFakeGateway();
  fake.program('RelayPluginViewMessage', undefined);
  setGateway(fake);
  await relayPluginViewMessage('tok1', { kind: 'ping', n: 1 });
  const call = fake.calls.find(c => c.method === 'RelayPluginViewMessage');
  assert(call?.args[0] === 'tok1', 'relayPluginViewMessage forwards the token unchanged'); // api.ts:1599
  assert(call?.args[1] === JSON.stringify({ kind: 'ping', n: 1 }), 'relayPluginViewMessage JSON.stringifies the message before the RPC'); // api.ts:1598
}

// relayPluginViewMessage: when absent, throws 'Plugin view relay is unavailable'.
{
  reset();
  setGateway(null);
  let threw: unknown = null;
  try {
    await relayPluginViewMessage('tok', { a: 1 });
  } catch (e) {
    threw = e;
  }
  assert(threw instanceof Error && threw.message === 'Plugin view relay is unavailable', 'relayPluginViewMessage throws when RelayPluginViewMessage is absent'); // api.ts:1594-1596
}

// preparePluginViewPanel: success returns the RPC's string result; absent throws
// the same 'Plugin view relay is unavailable' message.
{
  reset();
  const fake = createFakeGateway();
  fake.program('PreparePluginViewPanel', 'panel-token-xyz');
  setGateway(fake);
  const token = await preparePluginViewPanel('plug1', 'panel1');
  assert(token === 'panel-token-xyz', 'preparePluginViewPanel returns the token from PreparePluginViewPanel'); // api.ts:1587

  setGateway(null);
  let threw: unknown = null;
  try { await preparePluginViewPanel('plug1', 'panel1'); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'Plugin view relay is unavailable', 'preparePluginViewPanel throws when absent'); // api.ts:1584-1586
}

// releasePluginViewPanel: fire-and-forget; calls ReleasePluginViewPanel via
// optional chaining and is a silent no-op when the method (or app) is absent.
{
  reset();
  const fake = createFakeGateway();
  fake.program('ReleasePluginViewPanel', undefined);
  setGateway(fake);
  releasePluginViewPanel('tok1');
  assert(fake.calls.some(c => c.method === 'ReleasePluginViewPanel' && c.args[0] === 'tok1'), 'releasePluginViewPanel forwards the token to ReleasePluginViewPanel'); // api.ts:1603-1604

  setGateway(null);
  // Must not throw even though the app itself is null.
  releasePluginViewPanel('tok2'); // api.ts:1603 app?.ReleasePluginViewPanel?.(token)
}

// --- listPlugins / pingPlugin / setPluginEnabled ---------------------------

// api.ts:1206-1215 listPlugins: RPC failure is swallowed (handleError) and
// falls back to [].
{
  reset();
  const fake = createFakeGateway();
  fake.program('ListPlugins', () => { throw new Error('list failed'); });
  setGateway(fake);
  const plugins = await listPlugins();
  assert(Array.isArray(plugins) && plugins.length === 0, 'listPlugins falls back to [] on RPC failure'); // api.ts:1211-1213
  assert(get(lastError)?.message === 'List plugins: list failed', 'listPlugins reports failure via handleError without throwing'); // api.ts:1212
}

// api.ts:1217-1225 pingPlugin: RPC failure is fully swallowed (no rethrow,
// return type is void).
{
  reset();
  const fake = createFakeGateway();
  fake.program('PingPlugin', () => { throw new Error('ping failed'); });
  setGateway(fake);
  let threw: unknown = null;
  try { await pingPlugin('p1'); } catch (e) { threw = e; }
  assert(threw === null, 'pingPlugin does not rethrow on RPC failure'); // api.ts:1220-1223
  assert(get(lastError)?.message === 'Ping plugin: ping failed', 'pingPlugin still reports the failure via handleError'); // api.ts:1223
}

// api.ts:1227-1235 setPluginEnabled: RPC failure is swallowed, not rethrown.
{
  reset();
  const fake = createFakeGateway();
  fake.program('SetPluginEnabled', () => { throw new Error('toggle failed'); });
  setGateway(fake);
  let threw: unknown = null;
  try { await setPluginEnabled('p1', true); } catch (e) { threw = e; }
  assert(threw === null, 'setPluginEnabled does not rethrow on RPC failure'); // api.ts:1230-1234
  const call = fake.calls.find(c => c.method === 'SetPluginEnabled');
  assert(call?.args[0] === 'p1' && call.args[1] === true, 'setPluginEnabled forwards pluginId and enabled positionally'); // api.ts:1231
}

console.log('plugins.char.test passed');
