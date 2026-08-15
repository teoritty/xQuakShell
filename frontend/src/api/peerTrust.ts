// Atomic peer-trust RPC wrappers: the identities a plugin protocol asked about and the user
// confirmed. Thin wrappers around single backend calls, routed through callBackend for uniform
// error handling. No store access here.
//
// Answering a live prompt is not in this file - it belongs to a session and lives in
// api/sessions.ts next to resolveHostKeyRpc. These two are about the recorded list.
import { callBackend, callBackendVoid } from '../backend/callBackend';

/**
 * One confirmed remote identity, as the backend describes it.
 *
 * There is no material field. The backend keeps the bytes and renders the fingerprint from them,
 * so the screen can show which identity an entry is without ever holding one.
 */
export interface TrustedPeer {
  scope: string;
  subject: string;
  fingerprint: string;
  addedAt: string;
}

export async function getPeerTrust(): Promise<TrustedPeer[]> {
  return callBackend('Load trusted peers', [] as TrustedPeer[], async (app) => {
    return ((await app.GetPeerTrust()) ?? []) as TrustedPeer[];
  });
}

/**
 * Forget one confirmed identity, so the next connection asks again.
 *
 * Both halves of the coordinate are sent because an entry is keyed by (scope, subject): a plugin
 * trusting a host says nothing about another plugin trusting the same host, and deleting by
 * subject alone would silently revoke both.
 */
export async function removePeerTrust(scope: string, subject: string): Promise<void> {
  return callBackendVoid('Remove trusted peer', (app) => app.RemovePeerTrust(scope, subject));
}
