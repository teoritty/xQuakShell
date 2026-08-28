// Orchestration for the Plugins screen: the sequences that span more than one RPC.
//
// Single-RPC calls stay in api/* and are used directly by the components. What lives here is the
// composition - load the two lists that make up the screen, fetch a source's catalogue, or run an
// install and then refresh what the install changed - so no component has to remember that
// installing a plugin also invalidates the protocol cache.
import { listPlugins, type PluginInfo } from '../api/plugins';
import { listPluginSources, type PluginSourceDTO } from '../api/pluginSources';
import { fetchGitHubPlugins, type GitHubPluginMetadata } from '../api/githubPlugins';
import { showError } from '../stores/appState';
import {
  installGitHubPlugin,
  installPlugin,
  uninstallGitHubPlugin,
} from './protocolActions';
import type { ConsentAnswers } from '../lib/plugins/installConsent';

export interface PluginsScreenData {
  plugins: PluginInfo[];
  sources: PluginSourceDTO[];
}

/**
 * Loads both halves of the screen in parallel.
 *
 * A failure in either half is reported and downgraded to an empty list rather than rejecting: a
 * screen showing the installed plugins and no sources is more useful than a blank one, and the
 * two lists fail for unrelated reasons.
 */
export async function loadPluginsScreen(): Promise<PluginsScreenData> {
  const [plugins, sources] = await Promise.all([
    listPlugins(),
    listPluginSources().catch((e: unknown) => {
      const msg = e instanceof Error ? e.message : String(e);
      showError(`List plugin sources: ${msg}`, e instanceof Error && e.stack ? e.stack : '');
      return [] as PluginSourceDTO[];
    }),
  ]);
  return { plugins, sources };
}

/**
 * Reads what one source offers.
 *
 * Only forge sources are fetched. The marketplace has no listing to serve yet and its row already
 * carries the reason, so asking it would trade a rendered explanation for an error toast.
 */
export async function fetchSourceCatalog(
  source: PluginSourceDTO,
  forceRefresh = false,
): Promise<GitHubPluginMetadata[]> {
  if (source.kind !== 'forge' || !source.available) return [];
  const result = await fetchGitHubPlugins(source.id, forceRefresh);
  return result.plugins ?? [];
}

/** Installs from a registered source at a chosen release tag, then reports the new plugin list. */
export async function installFromSource(
  sourceId: string,
  releaseTag: string,
  consents: ConsentAnswers,
): Promise<PluginInfo[]> {
  await installGitHubPlugin(
    sourceId,
    releaseTag,
    consents.secret,
    consents.auth,
    consents.tunnel,
    consents.multiSession,
    consents.network,
    consents.exec,
  );
  return listPlugins();
}

/** Installs from a local folder or bundle path, then reports the new plugin list. */
export async function installFromPath(
  sourcePath: string,
  consents: ConsentAnswers,
): Promise<PluginInfo[]> {
  await installPlugin(
    sourcePath,
    consents.secret,
    consents.auth,
    consents.tunnel,
    consents.multiSession,
    consents.network,
    consents.exec,
  );
  return listPlugins();
}

/** Removes a plugin, optionally with its stored data, then reports the new plugin list. */
export async function removePlugin(pluginId: string, removeData: boolean): Promise<PluginInfo[]> {
  await uninstallGitHubPlugin(pluginId, removeData);
  return listPlugins();
}
