import {
  folders, connections, sessions, identities,
  vaultUnlocked, activeSessionId, transfers, transferCompleted, pendingHostKey,
  pingResults, platform,
  showError, editingFiles,
  type Session, type SessionEmbed,
  type RemoteNode, type TransferItem, type SSHIdentityMeta,
  type HostKeyEvent, type PingResult
} from './appState';
import { get, writable } from 'svelte/store';
import { getGateway, getRuntime } from '../backend/context';
import {
  importPassword,
  deletePassword,
  importIdentity,
  importPuTTYPPK,
  importPuTTYRegPreview,
  importPuTTYRegAsConnections,
  type PuTTYSessionPreview,
} from '../api/credentials';
import { addKnownHost, removeKnownHost } from '../api/knownHosts';
import { fetchSettings, putSettings, type AppSettings } from '../api/settings';
import {
  searchAuditLog,
  deleteAuditEntry,
  clearAuditLog,
  getAuditSessionState,
  enableAuditSecretLogging,
  disableAuditSecretLogging,
} from '../api/audit';
import { disposeTerminal } from '../lib/terminalPool';
import {
  resolveHostKeyRpc,
  getPlatform,
} from '../api/sessions';

export {
  reportEmbedViewport,
  reportEmbedActivity,
  getPlatform,
} from '../api/sessions';
export { unlockVault, lockVault } from '../actions/vaultActions';
export { sendTerminalInput, terminalResize } from '../api/terminal';
import {
  appendPendingTerminalOutput,
  clearPendingTerminalOutput,
  clearTerminalOutputConsumer,
  decodeTerminalOutput,
  hasTerminalOutputConsumer,
  registerTerminalOutputConsumer,
  takePendingTerminalOutput,
} from '../terminal/outputBuffer';

export {
  takePendingTerminalOutput,
  clearPendingTerminalOutput,
  registerTerminalOutputConsumer,
} from '../terminal/outputBuffer';
import {
  applyUiScalePercent,
  DEFAULT_UI_SCALE_PERCENT,
} from '../lib/uiScale';

export {
  type SessionHotkeysSettings,
  type AppSettings,
  type AuditEntry,
  type AuditSessionState,
  DEFAULT_SESSION_HOTKEYS,
} from '../api/settings';

function getApp(): any {
  return getGateway();
}

function getWailsRuntime(): any {
  return getRuntime();
}

function handleError(e: unknown, context?: string) {
  const msg = e instanceof Error ? e.message : String(e);
  const message = context ? `${context}: ${msg}` : msg;
  const details = e instanceof Error && e.stack ? e.stack : '';
  showError(message, details);
}

export {
  refreshFolders,
  saveFolder,
  deleteFolder,
  moveFolder,
  moveFolders,
  reorderFolders,
  createNewFolderInFolder,
} from '../actions/folderActions';
export {
  refreshAllConnections,
  refreshIdentities,
  saveConnection,
  deleteConnection,
  moveConnections,
  reorderConnections,
  createNewConnectionInFolder,
} from '../actions/connectionActions';

export { importPassword, deletePassword };

export {
  openSession,
  closeSession,
  createSessionFromSelection,
  focusNextSessionTab,
  focusPrevSessionTab,
  closeActiveSession,
} from '../actions/sessionActions';

export async function resolveHostKey(sessionId: string, action: string, host: string, authorizedKey: string): Promise<void> {
  return resolveHostKeyRpc(sessionId, action, host, authorizedKey);
}

import {
  listPath,
  uploadFile,
  downloadFile,
  cancelTransfer,
  removePath,
  mkdirPath,
  createFilePath,
  copyLocalPath,
  renamePath,
  chmodPath,
  chownPath,
  chmodPathRecursive,
  chownPathRecursive,
  type ApplyTarget,
} from '../api/remoteFs';
export {
  listPath,
  uploadFile,
  downloadFile,
  cancelTransfer,
  removePath,
  mkdirPath,
  createFilePath,
  copyLocalPath,
  renamePath,
  chmodPath,
  chownPath,
  chmodPathRecursive,
  chownPathRecursive,
  type ApplyTarget,
};

export {
  removeLocalPath,
  mkdirLocalPath,
  renameLocalPath,
  createLocalFile,
  selectLocalFile,
  selectLocalDirectory,
  listLocalPath,
  getPortableDataRoot,
  getUserHomeDir,
  getTempDir,
  openFileWithSystem,
  startFileWatch,
  type LocalNode,
} from '../api/localFs';

export {
  addKnownHost,
  removeKnownHost,
};

export {
  importIdentity,
  importPuTTYPPK,
  importPuTTYRegPreview,
  importPuTTYRegAsConnections,
  type PuTTYSessionPreview,
};

export { normalizeHotkey, parseHotkeyEvent } from '../hotkeys/hotkeys';

export async function getSettings(): Promise<AppSettings | null> {
  return fetchSettings();
}

export async function saveSettings(settings: Partial<AppSettings>): Promise<void> {
  return putSettings(settings);
}

export async function applyAppearanceSettings(): Promise<void> {
  const s = await getSettings();
  if (!s) return;
  applyUiScalePercent(s.uiScalePercent ?? DEFAULT_UI_SCALE_PERCENT);
}

export {
  searchAuditLog,
  deleteAuditEntry,
  clearAuditLog,
  getAuditSessionState,
  enableAuditSecretLogging,
  disableAuditSecretLogging,
};

// SFTPReady is a one-shot broadcast emitted once per session right after the
// remote filesystem is up. A FileTree component mounts only after its session
// reaches 'ready', so on fast (warm) connections the event can fire before the
// component subscribes — and a component that remounts (e.g. a tab dragged
// between tiles) would miss it too. We latch the readiness here at app-init,
// where a single always-on listener can never miss it, and expose it as a store
// so any (re)mounting FileTree can recover the session's ready state + initial
// path. Value = the session's initial remote path.
export const sftpReadyPaths = writable<Map<string, string>>(new Map());

export function subscribeToEvents(): void {
  const rt = getWailsRuntime();
  if (!rt) return;

  rt.EventsOn('SFTPReady', (data: { sessionId: string; initialPath?: string }) => {
    if (!data?.sessionId) return;
    sftpReadyPaths.update(m => {
      const next = new Map(m);
      next.set(data.sessionId, data.initialPath || '/');
      return next;
    });
  });

  rt.EventsOn('SessionStateChanged', (data: Session) => {
    if (data.state === 'closed') {
      sftpReadyPaths.update(m => {
        if (!m.has(data.sessionId)) return m;
        const next = new Map(m);
        next.delete(data.sessionId);
        return next;
      });
      disposeTerminal(data.sessionId);
    }
    sessions.update(list => {
      if (data.state === 'closed') {
        return list.filter(s => s.sessionId !== data.sessionId);
      }
      const idx = list.findIndex(s => s.sessionId === data.sessionId);
      if (idx >= 0) {
        list[idx] = { ...list[idx], ...data };
        return [...list];
      }
      return [...list, data];
    });
  });

  rt.EventsOn('TerminalOutput', (data: { sessionId: string; output: string }) => {
    if (!data?.sessionId) return;
    if (hasTerminalOutputConsumer(data.sessionId)) return;
    appendPendingTerminalOutput(data.sessionId, decodeTerminalOutput(data.output));
  });

  rt.EventsOn('SessionEmbedReady', (data: { sessionId: string; embed: SessionEmbed }) => {
    sessions.update(list => {
      const idx = list.findIndex(s => s.sessionId === data.sessionId);
      if (idx < 0) return list;
      list[idx] = {
        ...list[idx],
        surface: 'embed',
        embed: data.embed,
      };
      return [...list];
    });
  });

  rt.EventsOn('SessionClosed', (data: { sessionId: string }) => {
    clearPendingTerminalOutput(data.sessionId);
    clearTerminalOutputConsumer(data.sessionId);
    disposeTerminal(data.sessionId);
    sftpReadyPaths.update(m => {
      if (!m.has(data.sessionId)) return m;
      const next = new Map(m);
      next.delete(data.sessionId);
      return next;
    });
    sessions.update(list => list.filter(s => s.sessionId !== data.sessionId));
  });

  rt.EventsOn('TransferProgress', (data: TransferItem) => {
    // Byte transfers refresh trees only when they succeed; remote operations
    // (delete/chmod/chown) mutate the tree even on failure/cancel (partial
    // effect), so signal a refresh on any terminal state for those.
    const isOp = data.kind === 'delete' || data.kind === 'chmod' || data.kind === 'chown';
    const isTerminal = data.state === 'completed' || data.state === 'failed' || data.state === 'cancelled';
    const shouldRefresh = data.state === 'completed' || (isOp && isTerminal);
    transfers.update(list => {
      const idx = list.findIndex(t => t.id === data.id);
      if (idx >= 0) {
        list[idx] = { ...list[idx], ...data };
      } else {
        list = [...list, data];
      }
      if (shouldRefresh) {
        transferCompleted.set({ ...data });
      }
      return [...list];
    });
  });

  rt.EventsOn('HostKeyRequired', (data: HostKeyEvent) => {
    pendingHostKey.set(data);
  });

  rt.EventsOn('PingUpdated', (data: PingResult[]) => {
    const map = new Map<string, PingResult>();
    if (Array.isArray(data)) {
      for (const r of data) map.set(r.connectionId, r);
    }
    pingResults.set(map);
  });

  rt.EventsOn('VaultLocked', () => {
    vaultUnlocked.set(false);
    folders.set([]);
    connections.set([]);
    sessions.set([]);
    identities.set([]);
  });

  rt.EventsOn('FileEdited', (data: { localPath: string }) => {
    const path = data?.localPath;
    if (!path) return;
    editingFiles.update((map) => {
      const entry = map.get(path);
      if (entry) {
        uploadFile(entry.sessionId, path, entry.remotePath);
        const next = new Map(map);
        next.delete(path);
        return next;
      }
      return map;
    });
  });
}

export {
  type PluginInfo,
  type PluginInstallPreview,
  type PluginSettings,
  type PluginPublisherKeyPair,
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
} from '../api/plugins';
import type { PluginInfo } from '../api/plugins';
import { installPluginRpc } from '../api/plugins';
import {
  type GitHubRepository,
  type GitHubReleaseSummary,
  type GitHubPluginMetadata,
  type GitHubPluginList,
  type GitHubPluginPreview,
  listGitHubRepositories,
  addGitHubRepository,
  removeGitHubRepository,
  setGitHubRepositoryTrust,
  fetchGitHubPlugins,
  previewGitHubPluginInstall,
  installGitHubPluginRpc,
  uninstallGitHubPluginRpc,
} from '../api/githubPlugins';
export type {
  GitHubRepository,
  GitHubReleaseSummary,
  GitHubPluginMetadata,
  GitHubPluginList,
  GitHubPluginPreview,
} from '../api/githubPlugins';
export {
  listGitHubRepositories,
  addGitHubRepository,
  removeGitHubRepository,
  setGitHubRepositoryTrust,
  fetchGitHubPlugins,
  previewGitHubPluginInstall,
} from '../api/githubPlugins';

export interface FieldDef {
  id: string;
  label: string;
  type: 'text' | 'password' | 'number' | 'select' | 'checkbox' | 'textarea';
  required: boolean;
  default?: unknown;
  placeholder?: string;
  description?: string;
  width?: 'full' | 'half' | 'third';
  order: number;
  validation?: {
    minLength?: number;
    maxLength?: number;
    min?: number;
    max?: number;
    pattern?: string;
    maxSizeBytes?: number;
  };
  options?: { value: string; label: string }[];
  dependsOn?: string;
  secret: boolean;
}

export interface FieldGroup {
  id: string;
  label: string;
  order: number;
  fields: FieldDef[];
}

export interface ConnectionProtocol {
  id: string;
  label: string;
  defaultPort?: number;
  icon?: string;
  surface?: 'terminal' | 'embed';
  remoteFs?: boolean;
  fields?: FieldGroup[];
}

const DEFAULT_SSH_PROTOCOL: ConnectionProtocol = {
  id: 'ssh',
  label: 'SSH',
  defaultPort: 22,
  icon: 'terminal',
  remoteFs: true,
};

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
 * original combined installPlugin behavior. This shim is interim; Task 3.4
 * relocates the composition into actions/protocolActions.ts.
 */
export async function installPlugin(
  sourceDir: string,
  grantSecretAccess = false,
  grantAuthProviderAccess = false,
  grantTunnelProviderAccess = false,
  grantMultiSessionAccess = false,
  grantArbitraryNetworkAccess = false,
): Promise<PluginInfo> {
  const result = await installPluginRpc(sourceDir, grantSecretAccess, grantAuthProviderAccess, grantTunnelProviderAccess, grantMultiSessionAccess, grantArbitraryNetworkAccess);
  invalidateProtocolsCache();
  await refreshConnectionProtocols();
  return result;
}

/**
 * Composed public install: performs only the atomic install RPC
 * (installGitHubPluginRpc, in api/githubPlugins.ts). It does NOT refresh
 * the protocol cache — matching the original installGitHubPlugin behavior,
 * which never refreshed it either. This differs from uninstallGitHubPlugin
 * below, which does refresh the protocol cache. This shim is interim;
 * Task 3.4 relocates the composition into actions/protocolActions.ts.
 */
export async function installGitHubPlugin(
  repoURL: string,
  releaseTag = '',
  grantSecretAccess = false,
  grantAuthProviderAccess = false,
  grantTunnelProviderAccess = false,
  grantMultiSessionAccess = false,
  grantArbitraryNetworkAccess = false,
): Promise<void> {
  await installGitHubPluginRpc(repoURL, releaseTag, grantSecretAccess, grantAuthProviderAccess, grantTunnelProviderAccess, grantMultiSessionAccess, grantArbitraryNetworkAccess);
}

/**
 * Composed public uninstall: performs the atomic uninstall RPC
 * (uninstallGitHubPluginRpc, in api/githubPlugins.ts) then refreshes the
 * protocol cache — matching the original combined uninstallGitHubPlugin
 * behavior. This shim is interim; Task 3.4 relocates the composition into
 * actions/protocolActions.ts.
 */
export async function uninstallGitHubPlugin(pluginID: string, removeData = false): Promise<void> {
  await uninstallGitHubPluginRpc(pluginID, removeData);
  invalidateProtocolsCache();
  await refreshConnectionProtocols();
}

export {
  getPluginContributions,
  executePluginCommand,
  preparePluginViewPanel,
  relayPluginViewMessage,
  releasePluginViewPanel,
} from '../api/pluginRuntime';
export type {
  PluginCommand,
  PluginContributions,
  PluginAuthMethodContribution,
  PluginTunnelProviderContribution,
  PluginView,
  PluginStatusBarItem,
} from '../api/pluginRuntime';
