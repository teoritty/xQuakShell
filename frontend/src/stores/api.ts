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
import { disposeTerminal } from '../lib/terminalPool';
import { normalizeHotkey } from '../hotkeys/hotkeys';
import {
  applyUiScalePercent,
  DEFAULT_UI_SCALE_PERCENT,
  normalizeUiScalePercent,
} from '../lib/uiScale';

export interface SessionHotkeysSettings {
  create: string;
  next: string;
  prev: string;
  close: string;
}

export interface AppSettings {
  lockoutEnabled: boolean;
  lockoutIdleMinutes: number;
  lockOnMinimize: boolean;
  terminalFontFamily: string;
  terminalFontSize: number;
  terminalFontColor: string;
  theme: string;
  uiScalePercent: number;
  pingEnabled: boolean;
  pingMode: string;
  pingIntervalSeconds: number;
  pingIntervalMin: number;
  maxConcurrentPings: number;
  externalEditorPath: string;
  transferSpeedLimitKbps: number;
  connectionTimeoutSeconds: number;
  maxConcurrentTransfers: number;
  sessionHotkeyCreate: string;
  sessionHotkeyNext: string;
  sessionHotkeyPrev: string;
  sessionHotkeyClose: string;
  auditLogEnabled: boolean;
  auditRetentionMode: string;
  auditRetentionDays: number;
  auditRetentionCount: number;
  auditShowUsername: boolean;
  auditShowConnection: boolean;
  debugLogWindowEnabled: boolean;
}

export interface AuditEntry {
  id: number;
  timestamp: string;
  category: string;
  sessionId: string;
  connectionId: string;
  connectionName: string;
  host: string;
  username: string;
  input: string;
  redacted: boolean;
}

export interface AuditSessionState {
  logSecretsEnabled: boolean;
}

export const DEFAULT_SESSION_HOTKEYS: SessionHotkeysSettings = {
  create: 'Ctrl+Shift+N',
  next: 'Ctrl+Tab',
  prev: 'Ctrl+Shift+Tab',
  close: 'Ctrl+Shift+Q',
};

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
  try {
    const result: Folder[] = await app.GetFolders();
    folders.set(result || []);
  } catch (e) {
    handleError(e, 'Refresh folders');
  }
}

export async function refreshAllConnections(): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    const result: Connection[] = await app.GetAllConnections();
    connections.set(result || []);
  } catch (e) {
    handleError(e, 'Refresh connections');
  }
}

export async function refreshIdentities(): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    const result: SSHIdentityMeta[] = await app.GetIdentities();
    identities.set(result || []);
  } catch (e) {
    handleError(e, 'Refresh identities');
  }
}

export async function saveFolder(f: Partial<Folder>): Promise<Folder | null> {
  const app = getApp();
  if (!app) return null;
  try {
    const saved: Folder = await app.SaveFolder(f);
    await refreshFolders();
    return saved;
  } catch (e) {
    handleError(e, 'Save folder');
    return null;
  }
}

export async function deleteFolder(id: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.DeleteFolder(id);
    await refreshFolders();
    await refreshAllConnections();
  } catch (e) {
    handleError(e, 'Delete folder');
  }
}

export async function saveConnection(c: Partial<Connection>): Promise<Connection | null> {
  const app = getApp();
  if (!app) return null;
  try {
    const saved: Connection = await app.SaveConnection(c);
    await refreshAllConnections();
    return saved;
  } catch (e) {
    handleError(e, 'Save connection');
    return null;
  }
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
    await app.DeleteConnection(id);
    await refreshAllConnections();
  } catch (e) {
    handleError(e, 'Delete connection');
  }
}

export async function moveConnections(connectionIds: string[], targetFolderId: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.MoveConnections(connectionIds, targetFolderId);
    await refreshAllConnections();
  } catch (e) {
    handleError(e, 'Move connections');
  }
}

export async function moveFolder(folderId: string, targetParentId: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.MoveFolder(folderId, targetParentId);
    await refreshFolders();
  } catch (e) {
    handleError(e, 'Move folder');
  }
}

export async function moveFolders(folderIds: string[], targetParentId: string): Promise<void> {
  const app = getApp();
  if (!app || folderIds.length === 0) return;
  try {
    for (const folderId of folderIds) {
      await app.MoveFolder(folderId, targetParentId);
    }
    await refreshFolders();
  } catch (e) {
    handleError(e, 'Move folders');
  }
}

export async function reorderConnections(connectionIds: string[], folderId: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.ReorderConnections(connectionIds, folderId);
    await refreshAllConnections();
  } catch (e) {
    handleError(e, 'Reorder connections');
  }
}

export async function reorderFolders(folderIds: string[], parentId: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.ReorderFolders(folderIds, parentId);
    await refreshFolders();
  } catch (e) {
    handleError(e, 'Reorder folders');
  }
}

export async function importPassword(password: string, label: string): Promise<string> {
  const app = getApp();
  if (!app) return '';
  try {
    return await app.ImportPassword(password, label);
  } catch (e) {
    handleError(e, 'Import password');
    return '';
  }
}

export async function deletePassword(id: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.DeletePassword(id);
  } catch (e) {
    handleError(e, 'Delete password');
  }
}

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

export async function listPath(sessionId: string, path: string): Promise<RemoteNode[]> {
  const app = getApp();
  if (!app) return [];
  try {
    return await app.ListPath(sessionId, path);
  } catch (e) {
    handleError(e, 'List remote path');
    return [];
  }
}

function isCancelError(e: unknown): boolean {
  const msg = e instanceof Error ? e.message : String(e);
  return msg.toLowerCase().includes('cancel');
}

export async function uploadFile(sessionId: string, localPath: string, remotePath: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.Upload(sessionId, localPath, remotePath);
  } catch (e) {
    if (!isCancelError(e)) handleError(e, 'Upload file');
  }
}

export async function downloadFile(sessionId: string, remotePath: string, localPath: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.Download(sessionId, remotePath, localPath);
  } catch (e) {
    if (!isCancelError(e)) handleError(e, 'Download file');
  }
}

export function cancelTransfer(transferId: string): void {
  const app = getApp();
  if (!app) return;
  try {
    app.CancelTransfer(transferId);
  } catch (e) {
    handleError(e, 'Cancel transfer');
  }
}

export async function removePath(sessionId: string, path: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.RemovePath(sessionId, path);
  } catch (e) {
    handleError(e, 'Remove remote path');
  }
}

export async function mkdirPath(sessionId: string, parentPath: string, name: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.MkdirPath(sessionId, parentPath, name);
  } catch (e) {
    handleError(e, 'Create remote directory');
  }
}

export async function createFilePath(sessionId: string, parentPath: string, name: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.CreateFilePath(sessionId, parentPath, name);
  } catch (e) {
    handleError(e, 'Create remote file');
  }
}

export async function copyLocalPath(srcPath: string, destDir: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.CopyLocalPath(srcPath, destDir);
  } catch (e) {
    handleError(e, 'Copy local path');
  }
}

export async function renamePath(sessionId: string, oldPath: string, newPath: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.RenamePath(sessionId, oldPath, newPath);
  } catch (e) {
    handleError(e, 'Rename remote path');
  }
}

export type ApplyTarget = 'files' | 'dirs' | 'both';

export async function chmodPath(sessionId: string, path: string, mode: number): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.Chmod(sessionId, path, mode);
  } catch (e) {
    handleError(e, 'Change permissions');
  }
}

export async function chownPath(sessionId: string, path: string, uid: number, gid: number): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.Chown(sessionId, path, uid, gid);
  } catch (e) {
    handleError(e, 'Change owner');
  }
}

export async function chmodPathRecursive(sessionId: string, path: string, mode: number, applyTo: ApplyTarget): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.ChmodRecursive(sessionId, path, mode, applyTo);
  } catch (e) {
    handleError(e, 'Change permissions');
  }
}

export async function chownPathRecursive(sessionId: string, path: string, uid: number, gid: number, applyTo: ApplyTarget): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.ChownRecursive(sessionId, path, uid, gid, applyTo);
  } catch (e) {
    handleError(e, 'Change owner');
  }
}

export async function removeLocalPath(localPath: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.RemoveLocalPath(localPath);
  } catch (e) {
    handleError(e, 'Remove local path');
  }
}

export async function mkdirLocalPath(dirPath: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.MkdirLocalPath(dirPath);
  } catch (e) {
    handleError(e, 'Create local directory');
  }
}

export async function renameLocalPath(oldPath: string, newPath: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.RenameLocalPath(oldPath, newPath);
  } catch (e) {
    handleError(e, 'Rename local path');
  }
}

export async function createLocalFile(localPath: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.CreateLocalFile(localPath);
  } catch (e) {
    handleError(e, 'Create local file');
  }
}

export async function addKnownHost(host: string, keyBase64: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.AddKnownHost(host, keyBase64);
  } catch (e) {
    handleError(e, 'Add known host');
  }
}

export async function removeKnownHost(host: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.RemoveKnownHost(host);
  } catch (e) {
    handleError(e, 'Remove known host');
  }
}

export async function importIdentity(pemBase64: string, comment: string): Promise<string> {
  const app = getApp();
  if (!app) return '';
  try {
    return await app.ImportIdentity(pemBase64, comment);
  } catch (e) {
    handleError(e, 'Import identity');
    return '';
  }
}

export interface PuTTYSessionPreview {
  name: string;
  hostName: string;
  port: number;
  userName: string;
}

export async function importPuTTYPPK(ppkBase64: string, passphrase: string): Promise<string> {
  const app = getApp();
  if (!app) return '';
  try {
    return await app.ImportPuTTYPPK(ppkBase64, passphrase);
  } catch (e) {
    handleError(e, 'Import PPK');
    return '';
  }
}

export async function importPuTTYRegPreview(regContent: string): Promise<PuTTYSessionPreview[]> {
  const app = getApp();
  if (!app) return [];
  try {
    return await app.ImportPuTTYReg(regContent) || [];
  } catch (e) {
    handleError(e, 'Parse PuTTY REG');
    return [];
  }
}

export async function importPuTTYRegAsConnections(regContent: string, folderId: string): Promise<Connection[]> {
  const app = getApp();
  if (!app) return [];
  try {
    const result = await app.ImportPuTTYRegAsConnections(regContent, folderId) || [];
    return result as Connection[];
  } catch (e) {
    handleError(e, 'Import PuTTY sessions');
    return [];
  }
}

export async function selectLocalFile(): Promise<string> {
  const app = getApp();
  if (!app) return '';
  try {
    return await app.SelectLocalFile();
  } catch (e) {
    handleError(e, 'Select local file');
    return '';
  }
}

export async function selectLocalDirectory(): Promise<string> {
  const app = getApp();
  if (!app) return '';
  try {
    return await app.SelectLocalDirectory();
  } catch (e) {
    handleError(e, 'Select local directory');
    return '';
  }
}

export interface LocalNode {
  name: string;
  path: string;
  isDir: boolean;
  size: number;
  modTime?: string;
  mode?: string;
  owner?: string;
}

export async function listLocalPath(dirPath: string, includeHidden = false): Promise<LocalNode[]> {
  const app = getApp();
  if (!app) return [];
  try {
    return await app.ListLocalPath(dirPath, includeHidden);
  } catch (e) {
    handleError(e, 'List local path');
    return [];
  }
}

export async function getPortableDataRoot(): Promise<string> {
  const app = getApp();
  if (!app) return '';
  try {
    if (typeof app.GetPortableDataRoot === 'function') {
      return await app.GetPortableDataRoot();
    }
    return await app.GetUserHomeDir();
  } catch {
    return '';
  }
}

export async function getUserHomeDir(): Promise<string> {
  const app = getApp();
  if (!app) return '';
  try {
    return await app.GetUserHomeDir();
  } catch {
    return '';
  }
}

export async function getTempDir(): Promise<string> {
  const app = getApp();
  if (!app) return '';
  try {
    return await app.GetTempDir();
  } catch (e) {
    return '';
  }
}

export async function openFileWithSystem(localPath: string, editorPath?: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.OpenFileWithSystem(localPath, editorPath ?? '');
  } catch (e) {
    handleError(e, 'Open file');
  }
}

export function startFileWatch(localPath: string): void {
  const app = getApp();
  if (!app) return;
  try {
    app.StartFileWatch(localPath);
  } catch (e) {
    handleError(e, 'Start file watch');
  }
}

export { normalizeHotkey, parseHotkeyEvent } from '../hotkeys/hotkeys';

export async function getSettings(): Promise<AppSettings | null> {
  const app = getApp();
  if (!app) return null;
  try {
    const s: AppSettings = await app.GetSettings();
    s.sessionHotkeyCreate = normalizeHotkey(s.sessionHotkeyCreate || DEFAULT_SESSION_HOTKEYS.create);
    s.sessionHotkeyNext = normalizeHotkey(s.sessionHotkeyNext || DEFAULT_SESSION_HOTKEYS.next);
    s.sessionHotkeyPrev = normalizeHotkey(s.sessionHotkeyPrev || DEFAULT_SESSION_HOTKEYS.prev);
    s.sessionHotkeyClose = normalizeHotkey(s.sessionHotkeyClose || DEFAULT_SESSION_HOTKEYS.close);
    s.uiScalePercent = normalizeUiScalePercent(s.uiScalePercent);
    return s;
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    if (msg.toLowerCase().includes('vault is locked')) {
      // Expected during startup before unlock; avoid noisy modal.
      return null;
    }
    handleError(e, 'Get settings');
    return null;
  }
}

export async function saveSettings(settings: Partial<AppSettings>): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    const payload = {
      ...settings,
      sessionHotkeyCreate: normalizeHotkey(settings.sessionHotkeyCreate || DEFAULT_SESSION_HOTKEYS.create),
      sessionHotkeyNext: normalizeHotkey(settings.sessionHotkeyNext || DEFAULT_SESSION_HOTKEYS.next),
      sessionHotkeyPrev: normalizeHotkey(settings.sessionHotkeyPrev || DEFAULT_SESSION_HOTKEYS.prev),
      sessionHotkeyClose: normalizeHotkey(settings.sessionHotkeyClose || DEFAULT_SESSION_HOTKEYS.close),
    };
    await app.SaveSettings(payload);
  } catch (e) {
    handleError(e, 'Save settings');
  }
}

export async function applyAppearanceSettings(): Promise<void> {
  const s = await getSettings();
  if (!s) return;
  applyUiScalePercent(s.uiScalePercent ?? DEFAULT_UI_SCALE_PERCENT);
}

export async function searchAuditLog(
  query: string,
  sessionId: string,
  connectionId: string,
  category = '',
  limit = 200,
  offset = 0
): Promise<AuditEntry[]> {
  const app = getApp();
  if (!app?.SearchAuditLog) return [];
  try {
    return (await app.SearchAuditLog(query, sessionId, connectionId, category, limit, offset)) || [];
  } catch (e) {
    handleError(e, 'Search audit log');
    return [];
  }
}

export async function deleteAuditEntry(id: number): Promise<void> {
  const app = getApp();
  if (!app?.DeleteAuditEntry) return;
  try {
    await app.DeleteAuditEntry(id);
  } catch (e) {
    handleError(e, 'Delete audit entry');
  }
}

export async function clearAuditLog(category = ''): Promise<void> {
  const app = getApp();
  if (!app?.ClearAuditLog) return;
  try {
    await app.ClearAuditLog(category);
  } catch (e) {
    handleError(e, 'Clear audit log');
  }
}

export async function getAuditSessionState(): Promise<AuditSessionState | null> {
  const app = getApp();
  if (!app?.GetAuditSessionState) return null;
  try {
    return await app.GetAuditSessionState();
  } catch (e) {
    return null;
  }
}

export async function enableAuditSecretLogging(confirmed: boolean): Promise<boolean> {
  const app = getApp();
  if (!app?.EnableAuditSecretLogging) return false;
  try {
    await app.EnableAuditSecretLogging(confirmed);
    return true;
  } catch (e) {
    handleError(e, 'Enable audit secret logging');
    return false;
  }
}

export function disableAuditSecretLogging(): void {
  const app = getApp();
  if (!app?.DisableAuditSecretLogging) return;
  try {
    app.DisableAuditSecretLogging();
  } catch (e) {
    handleError(e, 'Disable audit secret logging');
  }
}

const MAX_PENDING_TERMINAL_BYTES = 256 << 10;
const pendingTerminalOutput = new Map<string, Uint8Array[]>();
const terminalOutputConsumers = new Map<string, number>();

// SFTPReady is a one-shot broadcast emitted once per session right after the
// remote filesystem is up. A FileTree component mounts only after its session
// reaches 'ready', so on fast (warm) connections the event can fire before the
// component subscribes — and a component that remounts (e.g. a tab dragged
// between tiles) would miss it too. We latch the readiness here at app-init,
// where a single always-on listener can never miss it, and expose it as a store
// so any (re)mounting FileTree can recover the session's ready state + initial
// path. Value = the session's initial remote path.
export const sftpReadyPaths = writable<Map<string, string>>(new Map());

function decodeTerminalOutput(output: string): Uint8Array {
  try {
    return Uint8Array.from(atob(output), (c) => c.charCodeAt(0));
  } catch {
    return new TextEncoder().encode(output);
  }
}

function appendPendingTerminalOutput(sessionId: string, bytes: Uint8Array): void {
  if (bytes.length === 0) return;
  let chunks = pendingTerminalOutput.get(sessionId);
  if (!chunks) {
    chunks = [];
    pendingTerminalOutput.set(sessionId, chunks);
  }
  chunks.push(bytes);
  let total = 0;
  for (const chunk of chunks) {
    total += chunk.length;
  }
  while (total > MAX_PENDING_TERMINAL_BYTES && chunks.length > 1) {
    const removed = chunks.shift()!;
    total -= removed.length;
  }
  if (total > MAX_PENDING_TERMINAL_BYTES && chunks.length === 1) {
    const overflow = total - MAX_PENDING_TERMINAL_BYTES;
    chunks[0] = chunks[0].slice(overflow);
  }
}

/** Returns buffered output emitted before the terminal component mounted. */
export function takePendingTerminalOutput(sessionId: string): Uint8Array[] {
  const chunks = pendingTerminalOutput.get(sessionId) ?? [];
  pendingTerminalOutput.delete(sessionId);
  return chunks;
}

export function clearPendingTerminalOutput(sessionId: string): void {
  pendingTerminalOutput.delete(sessionId);
}

/**
 * Marks a session as having a live terminal subscriber (skip global buffering).
 * Ref-counted: during a tile rearrangement the new Terminal component can mount
 * (and register) before the old one unmounts (and unregisters), so a plain flag
 * would briefly drop to "no consumer" and cause api.ts to buffer output that the
 * live terminal is already displaying — producing duplicated lines on the next
 * mount. Counting keeps the session marked as consumed throughout the overlap.
 */
export function registerTerminalOutputConsumer(sessionId: string): () => void {
  terminalOutputConsumers.set(sessionId, (terminalOutputConsumers.get(sessionId) ?? 0) + 1);
  let released = false;
  return () => {
    if (released) return;
    released = true;
    const next = (terminalOutputConsumers.get(sessionId) ?? 0) - 1;
    if (next <= 0) terminalOutputConsumers.delete(sessionId);
    else terminalOutputConsumers.set(sessionId, next);
  };
}

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
    if (terminalOutputConsumers.has(data.sessionId)) return;
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
    terminalOutputConsumers.delete(data.sessionId);
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

export interface PluginInfo {
  id: string;
  name: string;
  version: string;
  description: string;
  source: string;
  state: string;
  requiresSecretAccess: boolean;
  signed: boolean;
  enabled: boolean;
}

export interface PluginInstallPreview {
  id: string;
  name: string;
  version: string;
  description: string;
  signed: boolean;
  signatureVerified: boolean;
  checksumPresent: boolean;
  requiresSecretAccess: boolean;
  requiresAuthProviderAccess?: boolean;
  requiresTunnelProviderAccess?: boolean;
  multiSessionWarning?: boolean;
  arbitraryNetworkWarning?: boolean;
  unsignedWarning: boolean;
  untrustedSignatureWarning: boolean;
  permissions: string[];
}

export interface PluginSettings {
  trustedPublisherKeys: string[];
  requireSignedPlugins: boolean;
}

export interface PluginPublisherKeyPair {
  publicKey: string;
  privateKey: string;
}

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

export async function listPlugins(): Promise<PluginInfo[]> {
  const app = getApp();
  if (!app?.ListPlugins) return [];
  try {
    return await app.ListPlugins();
  } catch (e) {
    handleError(e, 'List plugins');
    return [];
  }
}

export async function pingPlugin(pluginId: string): Promise<void> {
  const app = getApp();
  if (!app?.PingPlugin) return;
  try {
    await app.PingPlugin(pluginId);
  } catch (e) {
    handleError(e, 'Ping plugin');
  }
}

export async function setPluginEnabled(pluginId: string, enabled: boolean): Promise<void> {
  const app = getApp();
  if (!app?.SetPluginEnabled) return;
  try {
    await app.SetPluginEnabled(pluginId, enabled);
  } catch (e) {
    handleError(e, 'Set plugin enabled');
  }
}

export async function selectPluginSourceDir(): Promise<string> {
  const app = getApp();
  if (!app?.SelectPluginSourceDir) return '';
  try {
    return await app.SelectPluginSourceDir();
  } catch (e) {
    handleError(e, 'Select plugin folder');
    return '';
  }
}

export async function selectPluginBundleFile(): Promise<string> {
  const app = getApp();
  if (!app?.SelectPluginBundleFile) return '';
  try {
    return await app.SelectPluginBundleFile();
  } catch (e) {
    handleError(e, 'Select plugin bundle');
    return '';
  }
}

export async function getPluginSettings(): Promise<PluginSettings> {
  const app = getApp();
  if (!app?.GetPluginSettings) {
    return { trustedPublisherKeys: [], requireSignedPlugins: false };
  }
  try {
    return await app.GetPluginSettings();
  } catch (e) {
    handleError(e, 'Load plugin settings');
    return { trustedPublisherKeys: [], requireSignedPlugins: false };
  }
}

export async function savePluginSettings(settings: PluginSettings): Promise<void> {
  const app = getApp();
  if (!app?.SavePluginSettings) return;
  try {
    await app.SavePluginSettings(settings);
  } catch (e) {
    handleError(e, 'Save plugin settings');
    throw e;
  }
}

export async function generatePluginPublisherKeyPair(): Promise<PluginPublisherKeyPair> {
  const app = getApp();
  if (!app?.GeneratePluginPublisherKeyPair) {
    return { publicKey: '', privateKey: '' };
  }
  try {
    return await app.GeneratePluginPublisherKeyPair();
  } catch (e) {
    handleError(e, 'Generate publisher keys');
    return { publicKey: '', privateKey: '' };
  }
}

export async function previewPluginInstall(sourceDir: string): Promise<PluginInstallPreview> {
  const app = getApp();
  if (!app?.PreviewPluginInstall) {
    throw new Error('Plugin install is unavailable');
  }
  return await app.PreviewPluginInstall(sourceDir);
}

export async function installPlugin(
  sourceDir: string,
  grantSecretAccess = false,
  grantAuthProviderAccess = false,
  grantTunnelProviderAccess = false,
  grantMultiSessionAccess = false,
  grantArbitraryNetworkAccess = false,
): Promise<PluginInfo> {
  const app = getApp();
  if (!app?.InstallPlugin) {
    throw new Error('Plugin install is unavailable');
  }
  try {
    const result = await app.InstallPlugin(sourceDir, grantSecretAccess, grantAuthProviderAccess, grantTunnelProviderAccess, grantMultiSessionAccess, grantArbitraryNetworkAccess);
    invalidateProtocolsCache();
    await refreshConnectionProtocols();
    return result;
  } catch (e) {
    handleError(e, 'Install plugin');
    throw e;
  }
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

export interface PluginCommand {
  pluginId: string;
  id: string;
  fullId: string;
  title: string;
  category?: string;
}

export interface PluginContributions {
  commands: PluginCommand[];
  views: PluginView[];
  statusBar: PluginStatusBarItem[];
  authMethods: PluginAuthMethodContribution[];
  tunnelProviders: PluginTunnelProviderContribution[];
}

export interface PluginAuthMethodContribution {
  pluginId: string;
  id: string;
  label: string;
  kind: string;
  fields?: FieldGroup[];
}

export interface PluginTunnelProviderContribution {
  pluginId: string;
  id: string;
  label: string;
}

export interface PluginView {
  pluginId: string;
  id: string;
  fullId: string;
  location: string;
  title: string;
  type?: string;
  entry?: string;
  assetUrl: string;
}

export interface PluginStatusBarItem {
  pluginId: string;
  id: string;
  text: string;
  tooltip?: string;
  priority?: number;
}

export async function getPluginContributions(): Promise<PluginContributions> {
  const app = getApp();
  if (!app?.GetPluginContributions) {
    return { commands: [], views: [], statusBar: [], authMethods: [], tunnelProviders: [] };
  }
  try {
    return await app.GetPluginContributions();
  } catch (e) {
    handleError(e, 'Load plugin contributions');
    return { commands: [], views: [], statusBar: [], authMethods: [], tunnelProviders: [] };
  }
}

export async function executePluginCommand(
  pluginId: string,
  commandId: string,
  args?: Record<string, unknown>
): Promise<Record<string, string>> {
  const app = getApp();
  if (!app?.ExecutePluginCommand) {
    throw new Error('Plugin commands are unavailable');
  }
  const rawArgs = args ? JSON.stringify(args) : null;
  const result = await app.ExecutePluginCommand(pluginId, commandId, rawArgs);
  if (!result) return {};
  if (typeof result === 'string') {
    try {
      return JSON.parse(result);
    } catch {
      return { message: result };
    }
  }
  return result as Record<string, string>;
}

export async function preparePluginViewPanel(pluginId: string, panelId: string): Promise<string> {
  const app = getApp();
  if (!app?.PreparePluginViewPanel) {
    throw new Error('Plugin view relay is unavailable');
  }
  return await app.PreparePluginViewPanel(pluginId, panelId);
}

export async function relayPluginViewMessage(
  token: string,
  message: Record<string, unknown>
): Promise<void> {
  const app = getApp();
  if (!app?.RelayPluginViewMessage) {
    throw new Error('Plugin view relay is unavailable');
  }
  const raw = JSON.stringify(message ?? {});
  await app.RelayPluginViewMessage(token, raw);
}

export function releasePluginViewPanel(token: string): void {
  const app = getApp();
  app?.ReleasePluginViewPanel?.(token);
}
