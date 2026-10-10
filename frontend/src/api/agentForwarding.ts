// The one backend call agent forwarding needs from the UI: answering a signature request.
import { callBackend } from '../backend/callBackend';

export type AgentDecision = 'once' | 'window' | 'deny';

/**
 * Answer a forwarded-agent signature request. The answer names the session as well as the
 * request, and the backend refuses it unless both match a request still waiting. Returns whether
 * the backend took it; a refusal is shown to the user by callBackend.
 */
export async function resolveAgentConfirmRpc(
  sessionId: string,
  requestId: string,
  decision: AgentDecision,
): Promise<boolean> {
  return callBackend('Answer signature request', false, async (app) => {
    await app.ResolveAgentConfirm(sessionId, requestId, decision);
    return true;
  });
}
