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

export interface PluginAuthConfig {
  pluginId: string;
  authMethodId: string;
  fields?: Record<string, string>;
}

export interface ForwardRule {
  id: string;
  kind: 'local' | 'remote' | 'dynamic';
  bindAddress: string;
  bindPort: number;
  targetHost?: string;
  targetPort?: number;
  pluginId?: string;
  providerId?: string;
  enabled: boolean;
}

export interface ConnectionUser {
  id: string;
  username: string;
  authMethod: 'key' | 'password' | 'plugin';
  keyAuth?: KeyAuthConfig;
  passAuth?: PassAuthConfig;
  pluginAuth?: PluginAuthConfig;
  label?: string;
}

export interface JumpHop {
  id: string;
  host: string;
  port: number;
  username: string;
  authMethod: 'key' | 'password' | 'plugin';
  keyAuth?: KeyAuthConfig;
  passAuth?: PassAuthConfig;
  pluginAuth?: PluginAuthConfig;
}

export interface Connection {
  id: string;
  folderId: string;
  name: string;
  host: string;
  port: number;
  order: number;
  users?: ConnectionUser[];
  defaultUserId?: string;
  tags?: string[];
  jumpChain?: JumpHop[];
  forwardRules?: ForwardRule[];
  protocol?: string;
  pluginFields?: Record<string, string>;
  // Plugin field ids whose secret value is stored in the vault. Their value is masked out of
  // pluginFields (secrets never reach the UI); the editor uses this to show a "saved" placeholder
  // and to keep an untouched secret out of the save payload so re-saving cannot wipe it.
  storedSecretFields?: string[];
}

export type SessionState = 'connecting' | 'hostkey-required' | 'ready' | 'error' | 'closed';

export interface SessionEmbed {
  uiUrl: string;
  tunnelUrl: string;
  sandbox?: string[];
}

export interface Session {
  sessionId: string;
  connectionId: string;
  connectionName: string;
  protocol?: string;
  surface?: 'terminal' | 'embed';
  embed?: SessionEmbed;
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

// TECH DEBT: this store/type is named "Transfer" but now represents any
// long-running operation (upload/download plus delete/chmod/chown — see `kind`).
// Kept for pragmatic reuse of the existing event→store→panel pipeline; a future
// refactor should rename to a generic "Operation" vocabulary.
export type OperationKind = 'upload' | 'download' | 'localcopy' | 'delete' | 'chmod' | 'chown';

export interface TransferItem {
  id: string;
  sessionId?: string;
  kind: OperationKind;
  localPath: string;
  /** Display caption for the panel row. Often path-shaped, but a batch reads
   *  "3 items" — never parse it as a path; use refreshDir. */
  remotePath: string;
  /** Directory to reload when the operation finishes. Always a real path and
   *  always populated by the backend (every emitter fills it), which is why
   *  there is no longer a fallback that derived it from remotePath. */
  refreshDir: string;
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

/**
 * Removes finished (completed/failed/cancelled) items from the transfers list,
 * keeping in-progress (active/pending) ones. Used by the panel's close button so
 * dismissing clears stale history instead of hiding it until the next event.
 */
export function clearFinishedTransfers(): void {
  transfers.update((list) => list.filter((t) => t.state === 'active' || t.state === 'pending'));
}

// NOTE: there is deliberately no removeTransfer(id) here. An operation's panel
// item belongs to the backend, which guarantees exactly one terminal event per
// op id on every exit path; deleting an item locally races that event, which
// would then re-create the item from scratch. The only way to retire a live
// item from the UI is to ask the backend to close it (cancelTransfer).

/** Emitted when a transfer completes; used to auto-refresh file trees. */
export const transferCompleted = writable<TransferItem | null>(null);
const EXPANDED_FOLDERS_KEY = 'xquakshell-expanded-folders';
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

const FAVORITES_KEY = 'xquakshell-favorites';
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

export const detailsConnectionId = writable<string>('');

export const detailsConnection = derived(
  [connections, detailsConnectionId],
  ([$connections, $detailsConnectionId]) =>
    $connections.find(c => c.id === $detailsConnectionId) || null
);

export const activeSession = derived(
  [sessions, activeSessionId],
  ([$sessions, $activeSessionId]) =>
    $sessions.find(s => s.sessionId === $activeSessionId) || null
);
