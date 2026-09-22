// Atomic session RPC wrappers. Matches the original stores/api.ts bodies'
// raw-RPC portions exactly.
//
// openSessionRpc / closeSessionRpc are deliberately atomic-only: unlike the
// original openSession/closeSession (which also drive the sessions and
// activeTabId stores optimistically, and in closeSession's case swallow
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

/**
 * Answer the host key prompt for a session. `action` is 'add' or 'replace'.
 *
 * The host and the key are deliberately NOT arguments: the backend reads them from the session's
 * own pending state. They used to be passed from here, which meant the frontend told the backend
 * which key to trust for which host - so anything reaching window.go could swap the recorded key
 * for a production host it had never connected to. The user's decision is the only part of this
 * the UI is entitled to supply.
 */
export async function resolveHostKeyRpc(sessionId: string, action: string): Promise<void> {
  const app = getGateway();
  if (!app) return;
  try {
    await app.ResolveHostKey(sessionId, action);
    pendingHostKey.set(null);
  } catch (e) {
    handleError(e, 'Resolve host key');
  }
}

/**
 * Hand the passphrase the user typed to the connection waiting on it.
 *
 * The request id is the whole address: which key it opens and which session uses it stay with the
 * waiting connection on the backend, so nothing here can aim a passphrase at a different key.
 * Returns whether the backend took it; the caller owns the dialog and closes it only then. The
 * passphrase is never put in the error shown to the user.
 */
export async function resolvePassphraseRpc(requestId: string, passphrase: string): Promise<boolean> {
  const app = getGateway();
  if (!app) return false;
  try {
    await app.ResolvePassphrase(requestId, passphrase);
    return true;
  } catch (e) {
    handleError(e, 'Unlock key');
    return false;
  }
}

/** Refuse a passphrase prompt; the connection waiting on it fails instead of hanging. */
export async function cancelPassphraseRpc(requestId: string): Promise<boolean> {
  const app = getGateway();
  if (!app) return false;
  try {
    await app.CancelPassphrase(requestId);
    return true;
  } catch (e) {
    handleError(e, 'Cancel key unlock');
    return false;
  }
}

/**
 * Answer the peer trust prompt for a session. `action` is 'trust' or 'reject'.
 *
 * Returns whether the backend accepted the decision, and touches no store of its own: the pending
 * prompt belongs to the component that shows it. A dialog dismissed on a call that failed would
 * leave the session waiting on an answer with nothing left on screen to give it.
 *
 * The subject and the material are not arguments, for the same reason they are not arguments to
 * resolveHostKeyRpc: the backend reads them from the session's own pending decision. The user's
 * answer is the only part of this the UI is entitled to supply.
 *
 * The fingerprint is the exception, and it travels the other way: it is what the dialog displayed,
 * echoed back so the backend can refuse an answer that no longer matches the pending question. It
 * is not a claim about what to trust - the backend compares it and stores its own copy either way.
 */
export async function resolvePeerTrustRpc(
  sessionId: string,
  action: string,
  fingerprint: string,
): Promise<boolean> {
  const app = getGateway();
  if (!app) return false;
  try {
    await app.ResolvePeerTrust(sessionId, action, fingerprint);
    return true;
  } catch (e) {
    handleError(e, 'Resolve peer trust');
    return false;
  }
}
