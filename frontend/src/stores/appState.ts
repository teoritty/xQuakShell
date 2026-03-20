import { writable, derived } from 'svelte/store';

export interface Folder {
  id: string;
  name: string;
  parentId: string;
  order: number;
}

export interface SSHIdentityMeta {
  id: string;
  comment: string;
  keyType: string;
}

export interface KeyAuthConfig {
  identityIds: string[];
}

export interface PassAuthConfig {
  passwordId: string;
}

export interface ConnectionUser {
  id: string;
  username: string;
  authMethod: 'key' | 'password';
  keyAuth?: KeyAuthConfig;
  passAuth?: PassAuthConfig;
  label?: string;
}

export interface JumpHop {
  host: string;
  port: number;
  username: string;
  authMethod: 'key' | 'password';
  keyAuth?: KeyAuthConfig;
  passAuth?: PassAuthConfig;
}

export interface ProxyConfig {
  type?: string;
  host: string;
  port: number;
  username?: string;
  passwordId?: string;
}

export interface TelnetConfig {
  host: string;
  port: number;
  username?: string;
  passwordId?: string;
}

export interface RDPConfig {
  host: string;
  port: number;
  username?: string;
  passwordId?: string;
  domain?: string;
}

export interface SerialConfig {
  port: string;
  baudRate: number;
  dataBits: number;
  stopBits: number;
  parity: string;
}

export interface HTTPConfig {
  url: string;
  method: string;
  auth?: string;
  passwordId?: string;
}

export interface Connection {
  id: string;
  folderId: string;
  name: string;
  host: string;
  port: number;
  order: number;
  user?: string;
  identityIds?: string[];
  users?: ConnectionUser[];
  defaultUserId?: string;
  tags?: string[];
  vpnProfileId?: string;
  jumpChain?: JumpHop[];
  proxy?: ProxyConfig;
  protocol?: string;
  telnetConfig?: TelnetConfig;
  rdpConfig?: RDPConfig;
  serialConfig?: SerialConfig;
  httpConfig?: HTTPConfig;
}

export type SessionState = 'connecting' | 'hostkey-required' | 'ready' | 'error' | 'closed';

export interface Session {
  sessionId: string;
  connectionId: string;
  connectionName: string;
  protocol?: string;
  state: SessionState;
  errorMessage: string;
}

export interface RemoteNode {
  path: string;
  name: string;
  isDir: boolean;
  size: number;
  modTime: string;
  mode?: string;
  owner?: string;
  group?: string;
}

export interface TransferItem {
  id: string;
  sessionId?: string;
  direction: 'upload' | 'download';
  localPath: string;
  remotePath: string;
  done: number;
  total: number;
  state: 'pending' | 'active' | 'completed' | 'failed' | 'cancelled';
}

export interface PingResult {
  connectionId: string;
  reachable: boolean;
  latencyMs: number;
}

export interface HostKeyEvent {
  sessionId: string;
  host: string;
  keyType: string;
  fingerprint: string;
  keyBase64: string;
  mismatch: boolean;
}

export const folders = writable<Folder[]>([]);
export const connections = writable<Connection[]>([]);
export const identities = writable<SSHIdentityMeta[]>([]);
export const selectedConnectionId = writable<string>('');
export const selectedConnectionIds = writable<Set<string>>(new Set());
export const selectedFolderId = writable<string>('');
export const sessions = writable<Session[]>([]);
export const activeSessionId = writable<string>('');
export const vaultUnlocked = writable<boolean>(false);
export const transfers = writable<TransferItem[]>([]);

/** Emitted when a transfer completes; used to auto-refresh file trees. */
export const transferCompleted = writable<TransferItem | null>(null);
const EXPANDED_FOLDERS_KEY = 'ssh-client-expanded-folders';
function loadExpandedFolders(): Set<string> {
  try {
    const raw = localStorage.getItem(EXPANDED_FOLDERS_KEY);
    if (raw) {
      const arr = JSON.parse(raw) as string[];
      return new Set(arr);
    }
  } catch {}
  return new Set();
}
function saveExpandedFolders(set: Set<string>) {
  try {
    localStorage.setItem(EXPANDED_FOLDERS_KEY, JSON.stringify([...set]));
  } catch {}
}
export const expandedFolderIds = writable<Set<string>>(loadExpandedFolders());
expandedFolderIds.subscribe(saveExpandedFolders);
export const pendingHostKey = writable<HostKeyEvent | null>(null);
export const pingResults = writable<Map<string, PingResult>>(new Map());
export const platform = writable<string>('');

const FAVORITES_KEY = 'ssh-client-favorites';
function loadFavorites(): Set<string> {
  try {
    const raw = localStorage.getItem(FAVORITES_KEY);
    if (raw) {
      const arr = JSON.parse(raw) as string[];
      return new Set(arr);
    }
  } catch {}
  return new Set();
}
function saveFavorites(set: Set<string>) {
  try {
    localStorage.setItem(FAVORITES_KEY, JSON.stringify([...set]));
  } catch {}
}
export const favorites = writable<Set<string>>(loadFavorites());
favorites.subscribe(saveFavorites);

export interface AppError {
  message: string;
  details: string;
}

export const lastError = writable<AppError | null>(null);

// Editing remote files: localPath -> { sessionId, remotePath } for auto re-upload on save
export const editingFiles = writable<Map<string, { sessionId: string; remotePath: string }>>(new Map());

export function showError(message: string, details?: string) {
  lastError.set({ message, details: details || '' });
}

export function clearError() {
  lastError.set(null);
}

export const selectedConnection = derived(
  [connections, selectedConnectionId],
  ([$connections, $selectedConnectionId]) =>
    $connections.find(c => c.id === $selectedConnectionId) || null
);

export const activeSession = derived(
  [sessions, activeSessionId],
  ([$sessions, $activeSessionId]) =>
    $sessions.find(s => s.sessionId === $activeSessionId) || null
);
