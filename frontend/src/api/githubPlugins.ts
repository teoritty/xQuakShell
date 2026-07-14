// Atomic GitHub-plugin RPC wrappers. Matches the original stores/api.ts
// bodies exactly.
//
// installGitHubPluginRpc / uninstallGitHubPluginRpc are deliberately
// atomic-only: unlike the original installGitHubPlugin/uninstallGitHubPlugin
// (which also invalidated/refreshed the protocol cache in the uninstall
// case), these functions perform ONLY their respective RPC. The
// protocol-refresh side effect is recomposed at the barrel level
// (stores/api.ts) for now, and will move to actions/protocolActions.ts in a
// later task.
import { getGateway } from '../backend/context';
import { showError } from '../stores/appState';

export interface GitHubRepository {
  url: string;
  owner: string;
  repo: string;
  displayName: string;
  addedAt: string;
  lastFetchedAt?: string;
  trusted: boolean;
}

export interface GitHubReleaseSummary {
  tag: string;
  name: string;
  publishedAt: string;
  prerelease: boolean;
  platformSupported: boolean;
  platforms: { os: string; arch: string; assetName: string }[];
}

export interface GitHubPluginMetadata {
  repositoryUrl: string;
  id: string;
  name: string;
  version: string;
  description: string;
  author: string;
  license: string;
  platforms: { os: string; arch: string; assetName: string }[];
  availableReleases: GitHubReleaseSummary[];
  latestRelease: string;
  prerelease: boolean;
  publishedAt: string;
  readme: string;
  minCoreVersion: string;
  platformSupported: boolean;
  installed: boolean;
  installedVersion: string;
  installedReleaseTag: string;
}

export interface GitHubPluginList {
  repositoryUrl: string;
  plugins: GitHubPluginMetadata[];
}

export interface GitHubPluginPreview {
  repositoryUrl: string;
  repositoryTrusted: boolean;
  id: string;
  name: string;
  version: string;
  description: string;
  author: string;
  license: string;
  minCoreVersion: string;
  currentPlatform: string;
  platformSupported: boolean;
  supportedPlatforms: string[];
  latestRelease: string;
  releaseTag: string;
  prerelease: boolean;
  publishedDate: string;
  readme: string;
  requiresSecretAccess: boolean;
  requiresAuthProviderAccess?: boolean;
  requiresTunnelProviderAccess?: boolean;
  multiSessionWarning?: boolean;
  arbitraryNetworkWarning: boolean;
  unsignedPlugin: boolean;
  untrustedSource: boolean;
  warnings: string[];
}

function handleError(e: unknown, context?: string) {
  const msg = e instanceof Error ? e.message : String(e);
  const message = context ? `${context}: ${msg}` : msg;
  const details = e instanceof Error && e.stack ? e.stack : '';
  showError(message, details);
}

export async function listGitHubRepositories(): Promise<GitHubRepository[]> {
  const app = getGateway();
  if (!app?.ListGitHubRepositories) return [];
  try {
    return await app.ListGitHubRepositories();
  } catch (e) {
    handleError(e, 'List GitHub repositories');
    return [];
  }
}

export async function addGitHubRepository(url: string, trusted: boolean): Promise<void> {
  const app = getGateway();
  if (!app?.AddGitHubRepository) throw new Error('GitHub repositories unavailable');
  try {
    await app.AddGitHubRepository({ url, trusted });
  } catch (e) {
    handleError(e, 'Add GitHub repository');
    throw e;
  }
}

export async function removeGitHubRepository(repoURL: string): Promise<void> {
  const app = getGateway();
  if (!app?.RemoveGitHubRepository) throw new Error('GitHub repositories unavailable');
  try {
    await app.RemoveGitHubRepository(repoURL);
  } catch (e) {
    handleError(e, 'Remove GitHub repository');
    throw e;
  }
}

export async function setGitHubRepositoryTrust(repoURL: string, trusted: boolean): Promise<void> {
  const app = getGateway();
  if (!app?.SetGitHubRepositoryTrust) throw new Error('GitHub repositories unavailable');
  try {
    await app.SetGitHubRepositoryTrust({ url: repoURL, trusted });
  } catch (e) {
    handleError(e, 'Update repository trust');
    throw e;
  }
}

export async function fetchGitHubPlugins(repoURL: string, forceRefresh = false): Promise<GitHubPluginList> {
  const app = getGateway();
  if (!app?.FetchGitHubPlugins) throw new Error('GitHub plugin discovery unavailable');
  try {
    return await app.FetchGitHubPlugins({ url: repoURL, forceRefresh });
  } catch (e) {
    handleError(e, 'Fetch GitHub plugins');
    throw e;
  }
}

export async function previewGitHubPluginInstall(repoURL: string, releaseTag = ''): Promise<GitHubPluginPreview> {
  const app = getGateway();
  if (!app?.PreviewGitHubPluginInstall) throw new Error('GitHub plugin install unavailable');
  try {
    return await app.PreviewGitHubPluginInstall(repoURL, releaseTag);
  } catch (e) {
    handleError(e, 'Preview GitHub plugin');
    throw e;
  }
}

/**
 * Atomic install RPC — performs ONLY the InstallGitHubPlugin call. Does NOT
 * invalidate/refresh the protocol cache; that composition lives at the
 * barrel level (stores/api.ts `installGitHubPlugin`) until Task 3.4
 * relocates it to actions/protocolActions.ts.
 */
export async function installGitHubPluginRpc(
  repoURL: string,
  releaseTag = '',
  grantSecretAccess = false,
  grantAuthProviderAccess = false,
  grantTunnelProviderAccess = false,
  grantMultiSessionAccess = false,
  grantArbitraryNetworkAccess = false,
): Promise<void> {
  const app = getGateway();
  if (!app?.InstallGitHubPlugin) throw new Error('GitHub plugin install unavailable');
  try {
    await app.InstallGitHubPlugin(repoURL, releaseTag, grantSecretAccess, grantAuthProviderAccess, grantTunnelProviderAccess, grantMultiSessionAccess, grantArbitraryNetworkAccess);
  } catch (e) {
    handleError(e, 'Install GitHub plugin');
    throw e;
  }
}

/**
 * Atomic uninstall RPC — performs ONLY the UninstallGitHubPlugin call. Does
 * NOT invalidate/refresh the protocol cache; that composition lives at the
 * barrel level (stores/api.ts `uninstallGitHubPlugin`) until Task 3.4
 * relocates it to actions/protocolActions.ts.
 */
export async function uninstallGitHubPluginRpc(pluginID: string, removeData = false): Promise<void> {
  const app = getGateway();
  if (!app?.UninstallGitHubPlugin) throw new Error('GitHub plugin uninstall unavailable');
  try {
    await app.UninstallGitHubPlugin(pluginID, removeData);
  } catch (e) {
    handleError(e, 'Uninstall plugin');
    throw e;
  }
}
