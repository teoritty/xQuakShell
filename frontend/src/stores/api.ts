import {
  folders, connections, sessions, identities,
  vaultUnlocked, activeSessionId, transfers, transferCompleted, pendingHostKey,
  selectedConnectionId, selectedFolderId, pingResults, platform,
  detailsConnectionId,
  showError, editingFiles,
  type Folder, type Connection, type Session,
  type RemoteNode, type TransferItem, type SSHIdentityMeta,
  type HostKeyEvent, type PingResult
} from './appState';
import { get } from 'svelte/store';
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
}

export interface AuditEntry {
  id: number;
  timestamp: string;
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

function getApp() {
  return (window as any).go?.main?.App;
}

function getWailsRuntime() {
  return (window as any).runtime;
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
    // terminal input errors are not shown in error dialog to avoid spam
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

export async function renamePath(sessionId: string, oldPath: string, newPath: string): Promise<void> {
  const app = getApp();
  if (!app) return;
  try {
    await app.RenamePath(sessionId, oldPath, newPath);
  } catch (e) {
    handleError(e, 'Rename remote path');
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

export function normalizeHotkey(input: string): string {
  if (!input) return '';
  const rawParts = input.split('+').map((x) => x.trim()).filter(Boolean);
  if (rawParts.length === 0) return '';
  const modifiers = new Set<string>();
  let key = '';
  for (const part of rawParts) {
    const lower = part.toLowerCase();
    if (lower === 'ctrl' || lower === 'control') modifiers.add('Ctrl');
    else if (lower === 'shift') modifiers.add('Shift');
    else if (lower === 'alt' || lower === 'option') modifiers.add('Alt');
    else if (lower === 'meta' || lower === 'cmd' || lower === 'win' || lower === 'super') modifiers.add('Meta');
    else if (lower === ' ') key = 'Space';
    else if (part.length === 1) key = part.toUpperCase();
    else key = part[0].toUpperCase() + part.slice(1);
  }
  const ordered: string[] = [];
  if (modifiers.has('Ctrl')) ordered.push('Ctrl');
  if (modifiers.has('Meta')) ordered.push('Meta');
  if (modifiers.has('Alt')) ordered.push('Alt');
  if (modifiers.has('Shift')) ordered.push('Shift');
  if (key) ordered.push(key);
  return ordered.join('+');
}

export function parseHotkeyEvent(e: KeyboardEvent): string {
  const parts: string[] = [];
  if (e.ctrlKey) parts.push('Ctrl');
  if (e.metaKey) parts.push('Meta');
  if (e.altKey) parts.push('Alt');
  if (e.shiftKey) parts.push('Shift');
  const ignoreKeys = new Set(['Control', 'Meta', 'Alt', 'Shift']);
  if (!ignoreKeys.has(e.key)) {
    if (e.key === ' ') parts.push('Space');
    else if (e.key.length === 1) parts.push(e.key.toUpperCase());
    else parts.push(e.key);
  }
  return normalizeHotkey(parts.join('+'));
}

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
  limit = 200,
  offset = 0
): Promise<AuditEntry[]> {
  const app = getApp();
  if (!app?.SearchAuditLog) return [];
  try {
    return (await app.SearchAuditLog(query, sessionId, connectionId, limit, offset)) || [];
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

export async function clearAuditLog(): Promise<void> {
  const app = getApp();
  if (!app?.ClearAuditLog) return;
  try {
    await app.ClearAuditLog();
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

export function subscribeToEvents(): void {
  const rt = getWailsRuntime();
  if (!rt) return;

  rt.EventsOn('SessionStateChanged', (data: Session) => {
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

  rt.EventsOn('SessionClosed', (data: { sessionId: string }) => {
    sessions.update(list => list.filter(s => s.sessionId !== data.sessionId));
  });

  rt.EventsOn('TransferProgress', (data: TransferItem) => {
    transfers.update(list => {
      const idx = list.findIndex(t => t.id === data.id);
      if (idx >= 0) {
        list[idx] = { ...list[idx], ...data };
        if (data.state === 'completed') {
          transferCompleted.set({ ...data });
        }
        return [...list];
      }
      const next = [...list, data];
      if (data.state === 'completed') {
        transferCompleted.set({ ...data });
      }
      return next;
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

export interface ConnectionProtocol {
  id: string;
  label: string;
  defaultPort?: number;
  icon?: string;
}

export async function getPluginConnectionProtocols(): Promise<ConnectionProtocol[]> {
  const app = getApp();
  if (!app?.GetPluginConnectionProtocols) {
    return [{ id: 'ssh', label: 'SSH', defaultPort: 22, icon: 'terminal' }];
  }
  try {
    return await app.GetPluginConnectionProtocols();
  } catch (e) {
    handleError(e, 'Load connection protocols');
    return [{ id: 'ssh', label: 'SSH', defaultPort: 22, icon: 'terminal' }];
  }
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
  grantMultiSessionAccess = false,
): Promise<PluginInfo> {
  const app = getApp();
  if (!app?.InstallPlugin) {
    throw new Error('Plugin install is unavailable');
  }
  try {
    return await app.InstallPlugin(sourceDir, grantSecretAccess, grantMultiSessionAccess);
  } catch (e) {
    handleError(e, 'Install plugin');
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
    return { commands: [], views: [], statusBar: [] };
  }
  try {
    return await app.GetPluginContributions();
  } catch (e) {
    handleError(e, 'Load plugin contributions');
    return { commands: [], views: [], statusBar: [] };
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
