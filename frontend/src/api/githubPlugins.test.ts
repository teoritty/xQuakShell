import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  listGitHubRepositories,
  addGitHubRepository,
  removeGitHubRepository,
  setGitHubRepositoryTrust,
  fetchGitHubPlugins,
  previewGitHubPluginInstall,
  installGitHubPluginRpc,
  uninstallGitHubPluginRpc,
} from './githubPlugins';
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

  // listGitHubRepositories: [] when absent
  let noMethod = withoutMethod(fake, 'ListGitHubRepositories');
  setGateway(noMethod);
  let repos = await listGitHubRepositories();
  assert(Array.isArray(repos) && repos.length === 0, 'listGitHubRepositories returns [] when absent');

  // listGitHubRepositories: swallows RPC failure, reports via lastError
  fake = createFakeGateway();
  fake.program('ListGitHubRepositories', () => { throw new Error('list failed'); });
  setGateway(fake);
  lastError.set(null);
  repos = await listGitHubRepositories();
  assert(Array.isArray(repos) && repos.length === 0, 'listGitHubRepositories falls back to [] on RPC failure');
  assert(get(lastError)?.message === 'List GitHub repositories: list failed', 'listGitHubRepositories reports failure via handleError');

  // addGitHubRepository: throws when absent
  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'AddGitHubRepository');
  setGateway(noMethod);
  let threw: unknown = null;
  try { await addGitHubRepository('https://example.com/repo', true); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'GitHub repositories unavailable', 'addGitHubRepository throws when absent');

  // addGitHubRepository: success forwards { url, trusted } as a single object arg
  fake = createFakeGateway();
  fake.program('AddGitHubRepository', undefined);
  setGateway(fake);
  await addGitHubRepository('https://example.com/repo', false);
  const addCall = fake.calls.find((c) => c.method === 'AddGitHubRepository');
  const addArg = addCall?.args[0] as { url: string; trusted: boolean };
  assert(addArg.url === 'https://example.com/repo' && addArg.trusted === false, 'addGitHubRepository packs url/trusted into a single object arg');

  // addGitHubRepository: on failure, sets lastError AND rethrows (does not swallow)
  fake = createFakeGateway();
  fake.program('AddGitHubRepository', () => { throw new Error('add failed'); });
  setGateway(fake);
  lastError.set(null);
  threw = null;
  try { await addGitHubRepository('https://example.com/repo', true); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'add failed', 'addGitHubRepository rethrows the original error');
  assert(get(lastError)?.message === 'Add GitHub repository: add failed', 'addGitHubRepository reports failure via handleError before rethrowing');

  // removeGitHubRepository: throws when absent; rethrows on failure
  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'RemoveGitHubRepository');
  setGateway(noMethod);
  threw = null;
  try { await removeGitHubRepository('url'); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'GitHub repositories unavailable', 'removeGitHubRepository throws when absent');

  fake = createFakeGateway();
  fake.program('RemoveGitHubRepository', () => { throw new Error('remove failed'); });
  setGateway(fake);
  lastError.set(null);
  threw = null;
  try { await removeGitHubRepository('url'); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'remove failed', 'removeGitHubRepository rethrows on failure');
  assert(get(lastError)?.message === 'Remove GitHub repository: remove failed', 'removeGitHubRepository reports via handleError');

  // setGitHubRepositoryTrust: throws when absent; forwards args; rethrows on failure
  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'SetGitHubRepositoryTrust');
  setGateway(noMethod);
  threw = null;
  try { await setGitHubRepositoryTrust('url', true); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'GitHub repositories unavailable', 'setGitHubRepositoryTrust throws when absent');

  fake = createFakeGateway();
  fake.program('SetGitHubRepositoryTrust', () => { throw new Error('trust failed'); });
  setGateway(fake);
  lastError.set(null);
  threw = null;
  try { await setGitHubRepositoryTrust('url', true); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'trust failed', 'setGitHubRepositoryTrust rethrows on failure');
  assert(get(lastError)?.message === 'Update repository trust: trust failed', 'setGitHubRepositoryTrust reports via handleError');

  // fetchGitHubPlugins: throws when absent; packs args; rethrows on failure
  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'FetchGitHubPlugins');
  setGateway(noMethod);
  threw = null;
  try { await fetchGitHubPlugins('url', true); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'GitHub plugin discovery unavailable', 'fetchGitHubPlugins throws when absent');

  fake = createFakeGateway();
  fake.program('FetchGitHubPlugins', () => { throw new Error('fetch failed'); });
  setGateway(fake);
  lastError.set(null);
  threw = null;
  try { await fetchGitHubPlugins('url', true); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'fetch failed', 'fetchGitHubPlugins rethrows on failure');
  assert(get(lastError)?.message === 'Fetch GitHub plugins: fetch failed', 'fetchGitHubPlugins reports via handleError');
  const fetchCall = fake.calls.find((c) => c.method === 'FetchGitHubPlugins');
  const fetchArg = fetchCall?.args[0] as { url: string; forceRefresh: boolean };
  assert(fetchArg.url === 'url' && fetchArg.forceRefresh === true, 'fetchGitHubPlugins packs url/forceRefresh into a single object arg');

  // previewGitHubPluginInstall: throws when absent; rethrows on failure
  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'PreviewGitHubPluginInstall');
  setGateway(noMethod);
  threw = null;
  try { await previewGitHubPluginInstall('url'); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'GitHub plugin install unavailable', 'previewGitHubPluginInstall throws when absent');

  fake = createFakeGateway();
  fake.program('PreviewGitHubPluginInstall', () => { throw new Error('preview failed'); });
  setGateway(fake);
  lastError.set(null);
  threw = null;
  try { await previewGitHubPluginInstall('url'); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'preview failed', 'previewGitHubPluginInstall rethrows on failure');
  assert(get(lastError)?.message === 'Preview GitHub plugin: preview failed', 'previewGitHubPluginInstall reports via handleError');

  // installGitHubPluginRpc: throws when absent
  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'InstallGitHubPlugin');
  setGateway(noMethod);
  threw = null;
  try { await installGitHubPluginRpc('url', 'v1', true, false, false, false, false); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'GitHub plugin install unavailable', 'installGitHubPluginRpc throws when absent');

  // installGitHubPluginRpc: on success forwards args positionally; does NOT
  // refresh protocols (atomic — no protocol-refresh side effect)
  fake = createFakeGateway();
  fake.program('InstallGitHubPlugin', undefined);
  fake.program('GetPluginConnectionProtocols', [{ id: 'ssh', label: 'SSH' }]);
  setGateway(fake);
  await installGitHubPluginRpc('url', 'v1', true, false, false, false, false);
  const installCall = fake.calls.find((c) => c.method === 'InstallGitHubPlugin');
  assert(
    !!installCall &&
    installCall.args[0] === 'url' && installCall.args[1] === 'v1' && installCall.args[2] === true &&
    installCall.args[3] === false && installCall.args[4] === false && installCall.args[5] === false && installCall.args[6] === false,
    'installGitHubPluginRpc forwards repoURL, releaseTag and grant flags positionally',
  );
  assert(!fake.calls.some((c) => c.method === 'GetPluginConnectionProtocols'), 'installGitHubPluginRpc does NOT trigger a protocols reload (atomic, no side effect)');

  // installGitHubPluginRpc: on failure, sets lastError AND rethrows (does not swallow)
  fake = createFakeGateway();
  fake.program('InstallGitHubPlugin', () => { throw new Error('install gh failed'); });
  setGateway(fake);
  lastError.set(null);
  threw = null;
  try { await installGitHubPluginRpc('url', 'v1', true, false, false, false, false); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'install gh failed', 'installGitHubPluginRpc rethrows on failure');
  assert(get(lastError)?.message === 'Install GitHub plugin: install gh failed', 'installGitHubPluginRpc reports failure via handleError before rethrowing');

  // uninstallGitHubPluginRpc: throws when absent
  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'UninstallGitHubPlugin');
  setGateway(noMethod);
  threw = null;
  try { await uninstallGitHubPluginRpc('p1'); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'GitHub plugin uninstall unavailable', 'uninstallGitHubPluginRpc throws when absent');

  // uninstallGitHubPluginRpc: on success forwards args; does NOT refresh protocols
  fake = createFakeGateway();
  fake.program('UninstallGitHubPlugin', undefined);
  fake.program('GetPluginConnectionProtocols', [{ id: 'ssh', label: 'SSH' }]);
  setGateway(fake);
  await uninstallGitHubPluginRpc('plugin-1', true);
  const uninstallCall = fake.calls.find((c) => c.method === 'UninstallGitHubPlugin');
  assert(!!uninstallCall && uninstallCall.args[0] === 'plugin-1' && uninstallCall.args[1] === true, 'uninstallGitHubPluginRpc forwards pluginID and removeData');
  assert(!fake.calls.some((c) => c.method === 'GetPluginConnectionProtocols'), 'uninstallGitHubPluginRpc does NOT trigger a protocols reload (atomic, no side effect)');

  // uninstallGitHubPluginRpc: on failure, sets lastError AND rethrows (does not swallow)
  fake = createFakeGateway();
  fake.program('UninstallGitHubPlugin', () => { throw new Error('uninstall failed'); });
  setGateway(fake);
  lastError.set(null);
  threw = null;
  try { await uninstallGitHubPluginRpc('p1'); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'uninstall failed', 'uninstallGitHubPluginRpc rethrows on failure');
  assert(get(lastError)?.message === 'Uninstall plugin: uninstall failed', 'uninstallGitHubPluginRpc reports failure via handleError before rethrowing');

  console.log('githubPlugins.test.ts passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
