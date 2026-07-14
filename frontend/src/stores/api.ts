import {
  folders, connections, sessions, identities,
  vaultUnlocked, activeSessionId, transfers, transferCompleted, pendingHostKey,
  selectedConnectionId, selectedFolderId, pingResults, platform,
  detailsConnectionId,
  showError, editingFiles,
  type Folder, type Connection, type Session, type SessionEmbed,
  type RemoteNode, type TransferItem, type SSHIdentityMeta,
  type HostKeyEvent, type PingResult
} from './appState';
import { get, writable } from 'svelte/store';
import { getGateway, getRuntime } from '../backend/context';
import {
  fetchFolders,
  putFolder,
  deleteFolderById,
  moveFolderTo,
  reorderFoldersIn,
} from '../api/folders';
import {
  fetchConnections,
  putConnection,
  deleteConnectionById,
  moveConnectionsTo,
  reorderConnectionsIn,
} from '../api/connections';
import {
  importPassword,
  deletePassword,
  fetchIdentities,
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

export async function unlockVault(masterPassword: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  await app.UnlockVault(masterPassword);
  vaultUnlocked.set(true);
  const p = await getPlatform();
  platform.set(p);
  await refreshFolders();
  await refreshAllConnections();
  await refreshIdentities();
  await refreshConnectionProtocols();
  await applyAppearanceSettings();
}

export async function lockVault(): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.LockVault();
  } catch (e) {
    handleError(e, 'Lock vault');
  }
  vaultUnlocked.set(false);
  folders.set([]);
  connections.set([]);
  sessions.set([]);
  identities.set([]);
}

export async function refreshFolders(): Promise<void> {
  const app = getApp();
  if (!app) return;
  const result = await fetchFolders();
  folders.set(result || []);
}

export async function refreshAllConnections(): Promise<void> {
  const app = getApp();
  if (!app) return;
  const result = await fetchConnections();
  connections.set(result || []);
}

export async function refreshIdentities(): Promise<void> {
  const app = getApp();
  if (!app) return;
  const result = await fetchIdentities();
  identities.set(result || []);
}

export async function saveFolder(f: Partial<Folder>): Promise<Folder | null> {
  const saved = await putFolder(f);
  if (saved) {
    await refreshFolders();
  }
  return saved;
}

export async function deleteFolder(id: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await deleteFolderById(id);
    await refreshFolders();
    await refreshAllConnections();
  } catch {
    // Error already reported by deleteFolderById via showError; skip refresh.
  }
}

export async function saveConnection(c: Partial<Connection>): Promise<Connection | null> {
  const saved = await putConnection(c);
  if (saved) {
    await refreshAllConnections();
  }
  return saved;
}

export async function createNewConnectionInFolder(folderId: string): Promise<Connection | null> {
  const uid = `u-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;
  const saved = await saveConnection({
    name: 'New connection',
    host: '',
    port: 22,
    folderId,
    users: [{ id: uid, username: '', authMethod: 'key' }],
    defaultUserId: uid,
  });
  if (saved) {
    selectedConnectionId.set(saved.id);
    detailsConnectionId.set(saved.id);
  }
  return saved;
}

export async function createNewFolderInFolder(parentId: string): Promise<void> {
  const saved = await saveFolder({
    name: 'New folder',
    parentId,
  });
  if (saved) {
    selectedFolderId.set(saved.id);
  }
}

export async function deleteConnection(id: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await deleteConnectionById(id);
    await refreshAllConnections();
  } catch {
    // Error already reported by deleteConnectionById via showError; skip refresh.
  }
}

export async function moveConnections(connectionIds: string[], targetFolderId: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await moveConnectionsTo(connectionIds, targetFolderId);
    await refreshAllConnections();
  } catch {
    // Error already reported by moveConnectionsTo via showError; skip refresh.
  }
}

export async function moveFolder(folderId: string, targetParentId: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await moveFolderTo(folderId, targetParentId);
    await refreshFolders();
  } catch {
    // Error already reported by moveFolderTo via showError; skip refresh.
  }
}

export async function moveFolders(folderIds: string[], targetParentId: string): Promise<void> {
  const app = getApp();
  if (!app || folderIds.length === 0) return;
  try {
    for (const folderId of folderIds) {
      await moveFolderTo(folderId, targetParentId, 'Move folders');
    }
    await refreshFolders();
  } catch {
    // Error already reported by moveFolderTo via showError; skip refresh.
  }
}

export async function reorderConnections(connectionIds: string[], folderId: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await reorderConnectionsIn(connectionIds, folderId);
    await refreshAllConnections();
  } catch {
    // Error already reported by reorderConnectionsIn via showError; skip refresh.
  }
}

export async function reorderFolders(folderIds: string[], parentId: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await reorderFoldersIn(folderIds, parentId);
    await refreshFolders();
  } catch {
    // Error already reported by reorderFoldersIn via showError; skip refresh.
  }
}

export { importPassword, deletePassword };

export async function openSession(connectionId: string): Promise<string | null> {
  const app = getApp();
  if (!app) return null;
  try {
    const sessionId: string = await app.OpenSession(connectionId);
    const conn = get(connections).find(c => c.id === connectionId);
    // Optimistic UI: show tab immediately, then backend events refine state.
    sessions.update((list) => {
      if (list.some((s) => s.sessionId === sessionId)) return list;
      return [
        ...list,
        {
          sessionId,
          connectionId,
          connectionName: conn?.name ?? 'Session',
          protocol: conn?.protocol ?? 'ssh',
          state: 'connecting',
          errorMessage: '',
        },
      ];
    });
    activeSessionId.set(sessionId);
    return sessionId;
  } catch (e) {
    handleError(e, 'Open session');
    return null;
  }
}

export async function closeSession(sessionId: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  // Optimistic UI: remove tab immediately so tree/tab status updates without waiting for the event round-trip.
  sessions.update((list) => list.filter((s) => s.sessionId !== sessionId));
  try {
    await app.CloseSession(sessionId);
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    if (msg.toLowerCase().includes('session not found')) {
      return;
    }
    handleError(e, 'Close session');
  }
}

export async function reportEmbedViewport(
  sessionId: string,
  widthPx: number,
  heightPx: number,
  devicePixelRatio: number,
): Promise<void> {
  const app = getApp();
  if (!app?.ReportEmbedViewport) return;
  try {
    await app.ReportEmbedViewport(sessionId, widthPx, heightPx, devicePixelRatio);
  } catch (e) {
    handleError(e, 'Report embed viewport');
  }
}

export async function reportEmbedActivity(sessionId: string, active: boolean): Promise<void> {
  const app = getApp();
  if (!app?.ReportEmbedActivity) return;
  try {
    await app.ReportEmbedActivity(sessionId, active);
  } catch (e) {
    handleError(e, 'Report embed activity');
  }
}

export async function createSessionFromSelection(): Promise<void> {
  const selectedId = get(selectedConnectionId);
  const allConnections = get(connections);
  const connectionId = selectedId || allConnections[0]?.id;
  if (!connectionId) return;
  await openSession(connectionId);
}

function cycleSession(direction: 1 | -1): void {
  const list = get(sessions);
  if (list.length === 0) return;
  const currentId = get(activeSessionId);
  const currentIdx = Math.max(0, list.findIndex((s) => s.sessionId === currentId));
  const nextIdx = (currentIdx + direction + list.length) % list.length;
  activeSessionId.set(list[nextIdx].sessionId);
}

export function focusNextSessionTab(): void {
  cycleSession(1);
}

export function focusPrevSessionTab(): void {
  cycleSession(-1);
}

export async function closeActiveSession(): Promise<void> {
  const currentId = get(activeSessionId);
  if (!currentId) return;
  await closeSession(currentId);
  const list = get(sessions);
  if (list.length > 0) {
    activeSessionId.set(list[list.length - 1].sessionId);
  } else {
    activeSessionId.set('');
  }
}

export async function getPlatform(): Promise<string> {
  const app = getApp();
  if (!app) return 'unknown';
  try {
    return await app.GetPlatform();
  } catch {
    return 'unknown';
  }
}

export async function resolveHostKey(sessionId: string, action: string, host: string, authorizedKey: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.ResolveHostKey(sessionId, action, host, authorizedKey);
    pendingHostKey.set(null);
  } catch (e) {
    handleError(e, 'Resolve host key');
  }
}

export async function sendTerminalInput(sessionId: string, data: string, commandLine = ''): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.SendTerminalInput(sessionId, data, commandLine);
  } catch (e) {
    console.debug('[terminal input]', sessionId, e);
  }
}

export async function terminalResize(sessionId: string, cols: number, rows: number): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.TerminalResize(sessionId, cols, rows);
  } catch (e) {
    // resize errors are non-critical
  }
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

export async function listGitHubRepositories(): Promise<GitHubRepository[]> {
  const app = getApp();
  if (!app?.ListGitHubRepositories) return [];
  try {
    return await app.ListGitHubRepositories();
  } catch (e) {
    handleError(e, 'List GitHub repositories');
    return [];
  }
}

export async function addGitHubRepository(url: string, trusted: boolean): Promise<void> {
  const app = getApp();
  if (!app?.AddGitHubRepository) throw new Error('GitHub repositories unavailable');
  try {
    await app.AddGitHubRepository({ url, trusted });
  } catch (e) {
    handleError(e, 'Add GitHub repository');
    throw e;
  }
}

export async function removeGitHubRepository(repoURL: string): Promise<void> {
  const app = getApp();
  if (!app?.RemoveGitHubRepository) throw new Error('GitHub repositories unavailable');
  try {
    await app.RemoveGitHubRepository(repoURL);
  } catch (e) {
    handleError(e, 'Remove GitHub repository');
    throw e;
  }
}

export async function setGitHubRepositoryTrust(repoURL: string, trusted: boolean): Promise<void> {
  const app = getApp();
  if (!app?.SetGitHubRepositoryTrust) throw new Error('GitHub repositories unavailable');
  try {
    await app.SetGitHubRepositoryTrust({ url: repoURL, trusted });
  } catch (e) {
    handleError(e, 'Update repository trust');
    throw e;
  }
}

export async function fetchGitHubPlugins(repoURL: string, forceRefresh = false): Promise<GitHubPluginList> {
  const app = getApp();
  if (!app?.FetchGitHubPlugins) throw new Error('GitHub plugin discovery unavailable');
  try {
    return await app.FetchGitHubPlugins({ url: repoURL, forceRefresh });
  } catch (e) {
    handleError(e, 'Fetch GitHub plugins');
    throw e;
  }
}

export async function previewGitHubPluginInstall(repoURL: string, releaseTag = ''): Promise<GitHubPluginPreview> {
  const app = getApp();
  if (!app?.PreviewGitHubPluginInstall) throw new Error('GitHub plugin install unavailable');
  try {
    return await app.PreviewGitHubPluginInstall(repoURL, releaseTag);
  } catch (e) {
    handleError(e, 'Preview GitHub plugin');
    throw e;
  }
}

export async function installGitHubPlugin(
  repoURL: string,
  releaseTag = '',
  grantSecretAccess = false,
  grantAuthProviderAccess = false,
  grantTunnelProviderAccess = false,
  grantMultiSessionAccess = false,
  grantArbitraryNetworkAccess = false,
): Promise<void> {
  const app = getApp();
  if (!app?.InstallGitHubPlugin) throw new Error('GitHub plugin install unavailable');
  try {
    await app.InstallGitHubPlugin(repoURL, releaseTag, grantSecretAccess, grantAuthProviderAccess, grantTunnelProviderAccess, grantMultiSessionAccess, grantArbitraryNetworkAccess);
  } catch (e) {
    handleError(e, 'Install GitHub plugin');
    throw e;
  }
}

export async function uninstallGitHubPlugin(pluginID: string, removeData = false): Promise<void> {
  const app = getApp();
  if (!app?.UninstallGitHubPlugin) throw new Error('GitHub plugin uninstall unavailable');
  try {
    await app.UninstallGitHubPlugin(pluginID, removeData);
    invalidateProtocolsCache();
    await refreshConnectionProtocols();
  } catch (e) {
    handleError(e, 'Uninstall plugin');
    throw e;
  }
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
