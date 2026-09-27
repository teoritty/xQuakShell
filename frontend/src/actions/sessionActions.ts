// Session orchestration layer: composes the atomic session RPCs
// (openSessionRpc / closeSessionRpc, in api/sessions.ts) with optimistic
// store updates. Moved verbatim from stores/api.ts (openSession, closeSession,
// createSessionFromSelection) except that the raw app.OpenSession /
// app.CloseSession calls are now routed through the atomic RPC wrappers, and
// the 'session not found' swallow (previously part of the atomic layer) is
// re-added here explicitly, since closeSessionRpc no longer performs it.
//
// Focus and close-by-hotkey moved to actions/tabActions.ts when a tab stopped
// meaning "a session": those verbs address whatever the tab bar shows, and this
// file addresses sessions.
import { get } from 'svelte/store';
import {
  sessions, connections, activeTabId, selectedConnectionId, showError, type Session,
} from '../stores/appState';
import { tileLayout } from '../stores/tileLayout';
import { openSessionRpc, closeSessionRpc } from '../api/sessions';
import { getGateway } from '../backend/context';
import { renameTab } from '../lib/tiles/operations';
import { transferPooledTerminal } from '../lib/terminalPool';
import { translate } from '../i18n/messages';

function handleError(e: unknown, context?: string) {
  const msg = e instanceof Error ? e.message : String(e);
  const message = context ? `${context}: ${msg}` : msg;
  const details = e instanceof Error && e.stack ? e.stack : '';
  showError(message, details);
}

export async function openSession(connectionId: string): Promise<string | null> {
  // Mirrors the original stores/api.ts guard: on a missing gateway, do
  // nothing observable (no store mutation, no error toast) and return null.
  if (!getGateway()) return null;
  try {
    const sessionId: string = await openSessionRpc(connectionId);
    const conn = get(connections).find((c) => c.id === connectionId);
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
    activeTabId.set(sessionId);
    return sessionId;
  } catch (e) {
    handleError(e, 'Open session');
    return null;
  }
}

export async function closeSession(sessionId: string): Promise<void> {
  // Mirrors the original stores/api.ts guard: on a missing gateway, do
  // nothing observable (no store mutation, no error toast) and return.
  if (!getGateway()) return;
  // Optimistic UI: remove tab immediately so tree/tab status updates without waiting for the event round-trip.
  sessions.update((list) => list.filter((s) => s.sessionId !== sessionId));
  try {
    await closeSessionRpc(sessionId);
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    if (msg.toLowerCase().includes('session not found')) {
      return;
    }
    handleError(e, 'Close session');
  }
}

export interface ReconnectOptions {
  /** Carry the old session's terminal over to the new one instead of starting empty. */
  preserveTerminal: boolean;
  /**
   * Called with the new session id before the new session enters the store, so a caller that
   * tracks sessions by id can recognise the replacement from its very first state update.
   */
  onOpened?: (newSessionId: string) => void;
}

/**
 * Replaces a session with a fresh one for the same connection, in the old one's place.
 *
 * The backend has no in-place reconnect: a session's transport, PTY, file system, forwards and
 * plugin state are all torn down and rebuilt, which is exactly what close + open already do along
 * one well-tested path. What a user sees as "the same tab" is assembled here instead: the new
 * session takes the old one's position in the tab list and in its tile, the focus if it had it,
 * and - when asked - its terminal.
 *
 * Returns the new session id, or null when nothing was replaced: no backend, the tab is gone, or
 * opening failed (reported to the user; the old session is left as it was).
 */
export async function reconnectSession(oldSessionId: string, opts: ReconnectOptions): Promise<string | null> {
  if (!getGateway()) return null;
  const old = get(sessions).find((s) => s.sessionId === oldSessionId);
  if (!old) return null;

  let newSessionId: string;
  try {
    newSessionId = await openSessionRpc(old.connectionId);
  } catch (e) {
    handleError(e, 'Reconnect session');
    return null;
  }
  if (!get(sessions).some((s) => s.sessionId === oldSessionId)) {
    // The user closed the tab while the new session was being opened. They are done with it, and a
    // session nobody asked for must not appear in its place.
    await closeQuietly(newSessionId);
    return null;
  }

  opts.onOpened?.(newSessionId);
  if (opts.preserveTerminal) {
    transferPooledTerminal(oldSessionId, newSessionId, reconnectBanner());
  }
  putInPlaceOf(old, newSessionId);
  await closeQuietly(oldSessionId);
  return newSessionId;
}

// Order matters. The tile is renamed first: reconcile runs on the sessions update below, and it
// must find the new id already placed, or it strips the old one - collapsing a tile it was alone
// in - and appends the new one to the active tile. The focus moves last, once the id it names is
// both placed and in the list.
function putInPlaceOf(old: Session, newSessionId: string): void {
  tileLayout.update((layout) => renameTab(layout, old.sessionId, newSessionId));
  sessions.update((list) => {
    // The new session's first state event may have arrived before its id did, in which case it is
    // already in the list with fresher state than anything assumed here.
    const arrived = list.find((s) => s.sessionId === newSessionId);
    const replacement: Session = arrived ?? {
      sessionId: newSessionId,
      connectionId: old.connectionId,
      connectionName: old.connectionName,
      protocol: old.protocol,
      state: 'connecting',
      errorMessage: '',
    };
    return list
      .filter((s) => s.sessionId !== newSessionId)
      .map((s) => (s.sessionId === old.sessionId ? replacement : s));
  });
  if (get(activeTabId) === old.sessionId) activeTabId.set(newSessionId);
}

async function closeQuietly(sessionId: string): Promise<void> {
  try {
    await closeSessionRpc(sessionId);
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    if (!msg.toLowerCase().includes('session not found')) handleError(e, 'Close session');
  }
}

// Dim, on a line of its own, so the output of the lost session and the prompt of the new one are
// never mistaken for each other.
function reconnectBanner(): string {
  return `\r\n\x1b[2m── ${translate('session.reconnected')} ──\x1b[0m\r\n`;
}

export async function createSessionFromSelection(): Promise<void> {
  const selectedId = get(selectedConnectionId);
  const allConnections = get(connections);
  const connectionId = selectedId || allConnections[0]?.id;
  if (!connectionId) return;
  await openSession(connectionId);
}

