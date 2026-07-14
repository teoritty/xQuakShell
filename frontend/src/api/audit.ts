// Atomic audit-log RPC wrappers. These guard on capability (method presence
// on the gateway) before calling, matching the original stores/api.ts bodies
// exactly, so they are implemented directly against getGateway/showError
// rather than through callBackend (which only guards on gateway presence).
import { getGateway } from '../backend/context';
import { showError } from '../stores/appState';
import type { AuditEntry, AuditSessionState } from './settings';

function handleError(e: unknown, context?: string) {
  const msg = e instanceof Error ? e.message : String(e);
  const message = context ? `${context}: ${msg}` : msg;
  const details = e instanceof Error && e.stack ? e.stack : '';
  showError(message, details);
}

export async function searchAuditLog(
  query: string,
  sessionId: string,
  connectionId: string,
  category = '',
  limit = 200,
  offset = 0
): Promise<AuditEntry[]> {
  const app = getGateway();
  if (!app?.SearchAuditLog) return [];
  try {
    return (await app.SearchAuditLog(query, sessionId, connectionId, category, limit, offset)) || [];
  } catch (e) {
    handleError(e, 'Search audit log');
    return [];
  }
}

export async function deleteAuditEntry(id: number): Promise<void> {
  const app = getGateway();
  if (!app?.DeleteAuditEntry) return;
  try {
    await app.DeleteAuditEntry(id);
  } catch (e) {
    handleError(e, 'Delete audit entry');
  }
}

export async function clearAuditLog(category = ''): Promise<void> {
  const app = getGateway();
  if (!app?.ClearAuditLog) return;
  try {
    await app.ClearAuditLog(category);
  } catch (e) {
    handleError(e, 'Clear audit log');
  }
}

export async function getAuditSessionState(): Promise<AuditSessionState | null> {
  const app = getGateway();
  if (!app?.GetAuditSessionState) return null;
  try {
    return await app.GetAuditSessionState();
  } catch (e) {
    return null;
  }
}

export async function enableAuditSecretLogging(confirmed: boolean): Promise<boolean> {
  const app = getGateway();
  if (!app?.EnableAuditSecretLogging) return false;
  try {
    await app.EnableAuditSecretLogging(confirmed);
    return true;
  } catch (e) {
    handleError(e, 'Enable audit secret logging');
    return false;
  }
}

export function disableAuditSecretLogging(): void {
  const app = getGateway();
  if (!app?.DisableAuditSecretLogging) return;
  try {
    app.DisableAuditSecretLogging();
  } catch (e) {
    handleError(e, 'Disable audit secret logging');
  }
}
