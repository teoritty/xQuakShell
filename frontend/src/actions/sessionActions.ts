// Session orchestration layer: composes the atomic session RPCs
// (openSessionRpc / closeSessionRpc, in api/sessions.ts) with optimistic
// store updates. Moved verbatim from stores/api.ts (openSession, closeSession,
// createSessionFromSelection, focusNextSessionTab, focusPrevSessionTab,
// closeActiveSession, cycleSession) except that the raw app.OpenSession /
// app.CloseSession calls are now routed through the atomic RPC wrappers, and
// the 'session not found' swallow (previously part of the atomic layer) is
// re-added here explicitly, since closeSessionRpc no longer performs it.
import { get } from 'svelte/store';
import { sessions, connections, activeSessionId, selectedConnectionId, showError } from '../stores/appState';
import { openSessionRpc, closeSessionRpc } from '../api/sessions';

function handleError(e: unknown, context?: string) {
  const msg = e instanceof Error ? e.message : String(e);
  const message = context ? `${context}: ${msg}` : msg;
  const details = e instanceof Error && e.stack ? e.stack : '';
  showError(message, details);
}

export async function openSession(connectionId: string): Promise<string | null> {
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
    activeSessionId.set(sessionId);
    return sessionId;
  } catch (e) {
    handleError(e, 'Open session');
    return null;
  }
}

export async function closeSession(sessionId: string): Promise<void> {
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
