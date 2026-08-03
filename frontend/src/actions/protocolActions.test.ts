import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  refreshConnectionProtocols,
  getPluginConnectionProtocols,
  invalidateProtocolsCache,
  connectionProtocolCatalogKey,
  connectionProtocols,
  installPlugin,
  installGitHubPlugin,
  uninstallGitHubPlugin,
  type ConnectionProtocol,
} from './protocolActions';
import { lastError } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

function reset() {
  lastError.set(null);
  invalidateProtocolsCache();
  connectionProtocols.set([]);
}

const PROTO_A: ConnectionProtocol[] = [{ id: 'ssh', label: 'SSH', fields: [] } as unknown as ConnectionProtocol];
const PROTO_B: ConnectionProtocol[] = [
  { id: 'ssh', label: 'SSH', fields: [] } as unknown as ConnectionProtocol,
  { id: 'ftp', label: 'FTP', fields: [] } as unknown as ConnectionProtocol,
];

async function run() {
  // --- getPluginConnectionProtocols warm-cache behavior -------------------

  {
    reset();
    const fake = createFakeGateway();
    fake.program('GetPluginConnectionProtocols', PROTO_A);
    setGateway(fake);

    const first = await getPluginConnectionProtocols();
    assert(first.length === 1 && first[0].id === 'ssh', 'cold cache issues an RPC and returns its result');
    assert(fake.calls.filter((c) => c.method === 'GetPluginConnectionProtocols').length === 1, 'cold call issues exactly one RPC');

    fake.program('GetPluginConnectionProtocols', PROTO_B);
    const second = await getPluginConnectionProtocols();
    assert(second.length === 1 && second[0].id === 'ssh', 'warm cache returns the previously cached list');
    assert(fake.calls.filter((c) => c.method === 'GetPluginConnectionProtocols').length === 1, 'warm cache issues no additional RPC');

    invalidateProtocolsCache();
    const third = await getPluginConnectionProtocols();
    assert(third.length === 2 && third[1].id === 'ftp', 'invalidateProtocolsCache forces a re-fetch');
    assert(fake.calls.filter((c) => c.method === 'GetPluginConnectionProtocols').length === 2, 'invalidation causes exactly one additional RPC');
  }

  // --- refreshConnectionProtocols: missing-gateway behavior ---------------

  {
    reset();
    connectionProtocols.set(PROTO_A);
    setGateway(null);
    const result = await refreshConnectionProtocols();
    assert(result.length === 1 && result[0].id === 'ssh', 'refreshConnectionProtocols with absent gateway returns the current store value');
    assert(get(connectionProtocols).length === 1, 'store is untouched when the gateway is absent');
  }

  // refreshConnectionProtocols: with the gateway absent, no cache write occurs either —
  // getPluginConnectionProtocols must still fall through to refreshConnectionProtocols.
  {
    reset();
    connectionProtocols.set(PROTO_A);
    setGateway(null);
    const again = await getPluginConnectionProtocols();
    assert(again.length === 1 && again[0].id === 'ssh', 'getPluginConnectionProtocols has no warm cache after an absent-gateway refresh');
  }

  // --- connectionProtocolCatalogKey ---------------------------------------

  {
    const key = connectionProtocolCatalogKey(PROTO_B);
    assert(key === 'ssh:0|ftp:0', 'connectionProtocolCatalogKey joins id:fieldCount pairs with |');
  }

  // --- installPlugin: asymmetry check (DOES refresh protocols) -----------

  {
    reset();
    const fake = createFakeGateway();
    fake.program('InstallPlugin', { id: 'p1', name: 'Plugin', enabled: true });
    fake.program('GetPluginConnectionProtocols', PROTO_B);
    setGateway(fake);

    const result = await installPlugin('/some/dir', true, false, false, false, false);
    assert((result as { id: string }).id === 'p1', 'installPlugin returns the object returned by InstallPlugin');
    const methods = fake.calls.map((c) => c.method);
    assert(methods[0] === 'InstallPlugin', 'installPlugin calls InstallPlugin first');
    assert(methods.includes('GetPluginConnectionProtocols'), 'installPlugin DOES refresh protocols after installing');
  }

  // installPlugin: missing InstallPlugin throws before any cache mutation.
  {
    reset();
    setGateway(null);
    let threw: unknown = null;
    try {
      await installPlugin('/dir');
    } catch (e) {
      threw = e;
    }
    assert(threw instanceof Error && threw.message === 'Plugin install is unavailable', 'installPlugin rethrows the atomic RPC missing-gateway error');
  }

  // --- installGitHubPlugin: asymmetry check (does NOT refresh protocols) --

  {
    reset();
    const fake = createFakeGateway();
    fake.program('InstallGitHubPlugin', undefined);
    fake.program('GetPluginConnectionProtocols', PROTO_B);
    setGateway(fake);

    await installGitHubPlugin('https://example.com/repo', 'v1', true, false, false, false, false);
    const methods = fake.calls.map((c) => c.method);
    assert(methods.includes('InstallGitHubPlugin'), 'installGitHubPlugin calls InstallGitHubPlugin');
    assert(!methods.includes('GetPluginConnectionProtocols'), 'installGitHubPlugin does NOT refresh protocols');
  }

  // installGitHubPlugin: missing gateway throws.
  {
    reset();
    setGateway(null);
    let threw: unknown = null;
    try {
      await installGitHubPlugin('https://example.com/repo');
    } catch (e) {
      threw = e;
    }
    assert(threw instanceof Error && threw.message === 'GitHub plugin install unavailable', 'installGitHubPlugin rethrows the atomic RPC missing-gateway error');
  }

  // --- uninstallGitHubPlugin: asymmetry check (DOES refresh protocols) ----

  {
    reset();
    connectionProtocols.set(PROTO_A);
    const warmFake = createFakeGateway();
    warmFake.program('GetPluginConnectionProtocols', PROTO_A);
    setGateway(warmFake);
    await getPluginConnectionProtocols();

    const fake = createFakeGateway();
    fake.program('UninstallGitHubPlugin', undefined);
    fake.program('GetPluginConnectionProtocols', PROTO_B);
    setGateway(fake);

    await uninstallGitHubPlugin('plugin-1', true);
    const methods = fake.calls.map((c) => c.method);
    assert(methods[0] === 'UninstallGitHubPlugin', 'uninstallGitHubPlugin calls UninstallGitHubPlugin first');
    assert(methods.includes('GetPluginConnectionProtocols'), 'uninstallGitHubPlugin DOES refresh protocols after uninstalling');
    assert(get(connectionProtocols).length === 2, 'the refreshed protocols store reflects the post-uninstall RPC result');
  }

  // uninstallGitHubPlugin: missing gateway throws.
  {
    reset();
    setGateway(null);
    let threw: unknown = null;
    try {
      await uninstallGitHubPlugin('p1');
    } catch (e) {
      threw = e;
    }
    assert(threw instanceof Error && threw.message === 'GitHub plugin uninstall unavailable', 'uninstallGitHubPlugin rethrows the atomic RPC missing-gateway error');
  }

  console.log('protocolActions.test passed');
}

run();
