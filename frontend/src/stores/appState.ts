import type { UpdateStatus } from '../api/update';
import { writable, derived } from 'svelte/store';

export interface Folder {
  id: string;
  name: string;
  parentId: string;
  order: number;
}

// The key manager's listing, as the connection editor needs it. The optional fields are the ones
// only schema 4 keys carry; a key skipped by the upgrade has none of them yet.
export interface SSHIdentityMeta {
  id: string;
  comment: string;
  keyType: string;
  fingerprint?: string;
  migrationPending?: boolean;
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
  /**
   * Acknowledges that a remote forward may bind beyond the SSH server's loopback. The backend
   * refuses such a rule without it, so a rule written straight into a saved connection cannot
   * publish a local service to the server's network unnoticed.
   */
  allowRemoteGateway?: boolean;
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

export type SessionState = 'connecting' | 'hostkey-required' | 'trust-required' | 'ready' | 'error' | 'closed';

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

/**
 * A session waiting on a decision about the remote peer's identity, raised by a plugin protocol.
 *
 * Deliberately not HostKeyEvent: that one carries an SSH key and its type, and the two dialogs must
 * stay separable. This one carries no key material at all - the backend keeps it and records it
 * from there, so the UI cannot name what it is trusting.
 */
export interface PeerTrustEvent {
  sessionId: string;
  subject: string;
  fingerprint: string;
  mismatch: boolean;
}

export const folders = writable<Folder[]>([]);
export const connections = writable<Connection[]>([]);
export const identities = writable<SSHIdentityMeta[]>([]);
export const selectedConnectionId = writable<string>('');
export const selectedConnectionIds = writable<Set<string>>(new Set());
/**
 * Where the next new folder or connection is created — "the folder the user is
 * currently pointing at", not "the folder row that is highlighted".
 *
 * INVARIANT: this store is a pure projection of the tree selection. Its only
 * writers are `syncSelectionStores` and `clearTreeSelection`
 * (lib/remoteTree/selection.ts), which both derive it through
 * `creationTargetFolderId`. Creating something must NEVER write here: a create
 * action that points the target at whatever it just made turns each click into
 * a child of the previous one, which is exactly how "New folder" used to build
 * a folder inside a folder inside a folder.
 */
export const creationTargetFolderId = writable<string>('');
export const sessions = writable<Session[]>([]);
/**
 * The id of the tab the user is looking at.
 *
 * A tab is EITHER an SSH session or a plugin-owned surface (ADR-015), so this
 * holds a sessionId or a surfaceId, and a reader has to say which it needs. It
 * was called activeSessionId until surfaces arrived; the name outlived the fact,
 * and three call sites ended up handing a `srf-…` id to a session-only backend
 * call because of it.
 *
 * Need a session specifically? Use `activeSession` below — it resolves to null
 * when the active tab is a surface. Need either? resolveTab in surfaceState.
 */
export const activeTabId = writable<string>('');
export const vaultUnlocked = writable<boolean>(false);
/**
 * Whether a vault file exists on disk, deciding between the create-master-password
 * and the unlock screen. `null` means the probe has not answered yet, so the gate
 * renders neither screen and the user never sees the wrong one flash.
 *
 * Locking does not reset this: a locked vault still exists.
 */
export const vaultExists = writable<boolean | null>(null);
/**
 * The one-time recovery key currently being shown, or null when no dialog is open.
 *
 * This is the only place in the frontend a key ever lives, and only while its dialog is on screen.
 * It is never persisted: the backend holds the copy that gets saved to a file, and both are dropped
 * the moment the user acknowledges it.
 */
export const pendingRecoveryKey = writable<string | null>(null);
/**
 * Set when the vault was opened with the recovery key, so the app shows the set-a-new-password
 * screen instead of the connection list. The vault is readable at that point, but the credential
 * that opened it is one the user was told to keep away from the machine.
 */
export const recoveryResetRequired = writable<boolean>(false);
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
/**
 * Trust questions waiting for an answer, oldest first.
 *
 * A queue rather than the single slot pendingHostKey uses, because two sessions can each be
 * stopped by one: with one slot the second question overwrites the first, and the first session
 * waits forever on a dialog that no longer exists. The backend already refuses to replace a live
 * question on one session; this is the same rule on the screen, across sessions.
 *
 * A question for a session that is already queued replaces that entry rather than appending: a
 * plugin retrying its handshake re-raises the same question, and two identical dialogs in a row is
 * how a user learns to dismiss them.
 */
export const pendingPeerTrust = writable<PeerTrustEvent[]>([]);

/** Queue a trust question, replacing any the same session already has. */
export function enqueuePeerTrust(event: PeerTrustEvent): void {
  pendingPeerTrust.update((queue) => [...queue.filter((q) => q.sessionId !== event.sessionId), event]);
}

/** Drop a session's trust question - answered, or the session is gone. */
export function dropPeerTrust(sessionId: string): void {
  pendingPeerTrust.update((queue) => queue.filter((q) => q.sessionId !== sessionId));
}
// updateStatus is the last release check the backend performed. It is a store rather than a
// per-component fetch because two places render it — the banner and the About panel — and a
// second fetch would report a check that never ran.
// The initial value is spelled out rather than imported from api/update: callBackend already
// imports this module, so pulling a runtime value the other way closes an import cycle and the
// constant is read before it is initialised.
export const updateStatus = writable<UpdateStatus>({
  currentVersion: '',
  latestVersion: '',
  releaseUrl: '',
  updateAvailable: false,
  checked: false,
});

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

/**
 * The active tab, resolved to a session — or null when it is a plugin surface.
 *
 * null is the whole point, not a degenerate case: it is the answer to "may I do
 * a session thing right now?", and anything that reaches a session-only backend
 * call (sending terminal input, re-running a command) must ask through here
 * rather than reading activeTabId and hoping.
 */
export const activeSession = derived(
  [sessions, activeTabId],
  ([$sessions, $activeTabId]) =>
    $sessions.find(s => s.sessionId === $activeTabId) || null
);
