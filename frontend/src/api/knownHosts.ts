// Atomic known-hosts RPC wrappers. Thin wrappers around single backend RPC
// calls, routed through callBackend for uniform error handling. No store
// access here.
import { callBackendVoid } from '../backend/callBackend';

export async function removeKnownHost(host: string): Promise<void> {
  return callBackendVoid('Remove known host', (app) => app.RemoveKnownHost(host));
}
