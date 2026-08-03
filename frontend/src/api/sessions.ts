// Atomic session RPC wrappers. Matches the original stores/api.ts bodies'
// raw-RPC portions exactly.
//
// openSessionRpc / closeSessionRpc are deliberately atomic-only: unlike the
// original openSession/closeSession (which also drive the sessions and
// activeSessionId stores optimistically, and in closeSession's case swallow
// a "session not found" error), these functions perform ONLY their
// respective RPC call. That orchestration remains in stores/api.ts
// (openSession/closeSession) until a later task relocates it to actions/.
import { getGateway } from '../backend/context';
import { showError, pendingHostKey } from '../stores/appState';

function handleError(e: unknown, context?: string) {
  const msg = e instanceof Error ? e.message : String(e);
  const message = context ? `${context}: ${msg}` : msg;
  const details = e instanceof Error && e.stack ? e.stack : '';
  showError(message, details);
}

export async function openSessionRpc(connectionId: string): Promise<string> {
  const app = getGateway();
  if (!app) throw new Error('Backend unavailable');
  return await app.OpenSession(connectionId);
}

/**
 * Atomic close RPC — performs ONLY the CloseSession call and does NOT
 * swallow a "session not found" error. The swallow is an orchestration
 * concern that belongs to the still-in-place closeSession wrapper in
 * stores/api.ts (and, later, sessionActions) — this atomic version lets
 * every error, including "session not found", propagate to the caller.
 */
export async function closeSessionRpc(sessionId: string): Promise<void> {
  const app = getGateway();
  if (!app) return;
  await app.CloseSession(sessionId);
}

export async function reportEmbedViewport(
  sessionId: string,
  widthPx: number,
  heightPx: number,
  devicePixelRatio: number,
): Promise<void> {
  const app = getGateway();
  if (!app?.ReportEmbedViewport) return;
  try {
    await app.ReportEmbedViewport(sessionId, widthPx, heightPx, devicePixelRatio);
  } catch (e) {
    handleError(e, 'Report embed viewport');
  }
}

export async function reportEmbedActivity(sessionId: string, active: boolean): Promise<void> {
  const app = getGateway();
  if (!app?.ReportEmbedActivity) return;
  try {
    await app.ReportEmbedActivity(sessionId, active);
  } catch (e) {
    handleError(e, 'Report embed activity');
  }
}

export async function getPlatform(): Promise<string> {
  const app = getGateway();
  if (!app) return 'unknown';
  try {
    return await app.GetPlatform();
  } catch {
    return 'unknown';
  }
}

export async function resolveHostKeyRpc(sessionId: string, action: string, host: string, authorizedKey: string): Promise<void> {
  const app = getGateway();
  if (!app) return;
  try {
    await app.ResolveHostKey(sessionId, action, host, authorizedKey);
    pendingHostKey.set(null);
  } catch (e) {
    handleError(e, 'Resolve host key');
  }
}
