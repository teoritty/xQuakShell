// Connection-protocol orchestration layer: owns the `connectionProtocols`
// store + `protocolsCache` module state and composes the atomic plugin
// install/uninstall RPCs (installPluginRpc in api/plugins.ts;
// installGitHubPluginRpc / uninstallGitHubPluginRpc in api/githubPlugins.ts)
// with protocol-cache invalidation/refresh. Moved verbatim from
// stores/api.ts (including the interim shims installPlugin /
// installGitHubPlugin / uninstallGitHubPlugin added in Task 2.6b/2.6c).
//
// Missing-gateway guard analysis (per-function, matches the original
// stores/api.ts bodies exactly):
// - refreshConnectionProtocols: original guarded with `getApp()` (here
//   `getGateway()`) before touching the cache/store — on a missing method it
//   returns the current connectionProtocols store value and performs no
//   cache write. Reproduced explicitly below.
// - getPluginConnectionProtocols: no explicit guard of its own; it is
//   cache-first and falls through to refreshConnectionProtocols (which has
//   its own guard) on a cold cache.
// - invalidateProtocolsCache / connectionProtocolCatalogKey: pure, no
//   gateway interaction, no guard needed.
// - installPlugin / installGitHubPlugin / uninstallGitHubPlugin: the
//   original had NO explicit `getApp()` guard of its own; each relies on the
//   underlying atomic RPC wrapper (installPluginRpc / installGitHubPluginRpc
//   / uninstallGitHubPluginRpc) throwing "* is unavailable" synchronously
//   when the corresponding method is absent on the gateway, before any
//   cache/store mutation runs. No guard is added here — reproducing that
//   throw-before-mutate behavior exactly.
import { get, writable } from 'svelte/store';
import { getGateway } from '../backend/context';
import { showError } from '../stores/appState';
import { installPluginRpc, type PluginInfo } from '../api/plugins';
import { installGitHubPluginRpc, uninstallGitHubPluginRpc } from '../api/githubPlugins';
import type { ConnectionProtocol, FieldGroup, FieldDef } from '../api/protocolTypes';

// Re-exported for compatibility with existing importers that reference these
// domain types via actions/protocolActions (the canonical definitions now
// live in api/protocolTypes.ts).
export type { ConnectionProtocol, FieldGroup, FieldDef };

const DEFAULT_SSH_PROTOCOL: ConnectionProtocol = {
  id: 'ssh',
  label: 'SSH',
  defaultPort: 22,
  icon: 'terminal',
  remoteFs: true,
};

function getApp(): any {
  return getGateway();
}

function handleError(e: unknown, context?: string) {
  const msg = e instanceof Error ? e.message : String(e);
  const message = context ? `${context}: ${msg}` : msg;
  const details = e instanceof Error && e.stack ? e.stack : '';
  showError(message, details);
}

/** Live protocol catalog for connection editor and session chrome. */
export const connectionProtocols = writable<ConnectionProtocol[]>([DEFAULT_SSH_PROTOCOL]);

let protocolsCache: ConnectionProtocol[] | null = null;

function protocolCatalogKey(list: ConnectionProtocol[]): string {
  return list.map((p) => `${p.id}:${p.fields?.length ?? 0}`).join('|');
}

/** Returns a stable signature of the protocol catalog (ids + field counts). */
export function connectionProtocolCatalogKey(list: ConnectionProtocol[]): string {
  return protocolCatalogKey(list);
}

/** Reloads plugin connection protocols from the backend and updates {@link connectionProtocols}. */
export async function refreshConnectionProtocols(): Promise<ConnectionProtocol[]> {
  const app = getApp();
  if (!app?.GetPluginConnectionProtocols) {
    // Wails may not be bound yet — keep the current store value and do not cache.
    return get(connectionProtocols);
  }
  try {
    const list = await app.GetPluginConnectionProtocols();
    protocolsCache = list;
    connectionProtocols.set(list);
    return list;
  } catch (e) {
    handleError(e, 'Load connection protocols');
    return get(connectionProtocols);
  }
}

export async function getPluginConnectionProtocols(): Promise<ConnectionProtocol[]> {
  if (protocolsCache) {
    return protocolsCache;
  }
  return refreshConnectionProtocols();
}

export function invalidateProtocolsCache(): void {
  protocolsCache = null;
}

/**
 * Composed public install: performs the atomic install RPC (installPluginRpc,
 * in api/plugins.ts) then refreshes the protocol cache — matching the
 * original combined installPlugin behavior.
 */
export async function installPlugin(
  sourceDir: string,
  grantSecretAccess = false,
  grantAuthProviderAccess = false,
  grantTunnelProviderAccess = false,
  grantMultiSessionAccess = false,
  grantArbitraryNetworkAccess = false,
  grantExecAccess = false,
): Promise<PluginInfo> {
  const result = await installPluginRpc(sourceDir, grantSecretAccess, grantAuthProviderAccess, grantTunnelProviderAccess, grantMultiSessionAccess, grantArbitraryNetworkAccess, grantExecAccess);
  invalidateProtocolsCache();
  await refreshConnectionProtocols();
  return result;
}

/**
 * Composed public install: performs only the atomic install RPC
 * (installGitHubPluginRpc, in api/githubPlugins.ts). It does NOT refresh
 * the protocol cache — matching the original installGitHubPlugin behavior,
 * which never refreshed it either. This differs from uninstallGitHubPlugin
 * below, which does refresh the protocol cache.
 */
export async function installGitHubPlugin(
  repoURL: string,
  releaseTag = '',
  grantSecretAccess = false,
  grantAuthProviderAccess = false,
  grantTunnelProviderAccess = false,
  grantMultiSessionAccess = false,
  grantArbitraryNetworkAccess = false,
  grantExecAccess = false,
): Promise<void> {
  await installGitHubPluginRpc(repoURL, releaseTag, grantSecretAccess, grantAuthProviderAccess, grantTunnelProviderAccess, grantMultiSessionAccess, grantArbitraryNetworkAccess, grantExecAccess);
}

/**
 * Composed public uninstall: performs the atomic uninstall RPC
 * (uninstallGitHubPluginRpc, in api/githubPlugins.ts) then refreshes the
 * protocol cache — matching the original combined uninstallGitHubPlugin
 * behavior.
 */
export async function uninstallGitHubPlugin(pluginID: string, removeData = false): Promise<void> {
  await uninstallGitHubPluginRpc(pluginID, removeData);
  invalidateProtocolsCache();
  await refreshConnectionProtocols();
}
