// Atomic connection RPC wrappers. Each function is a thin wrapper around a
// single backend RPC call, routed through callBackend/callBackendVoid for
// uniform error handling. No store access here — orchestration lives in
// actions/connectionActions.ts, which combines these with `connections` store
// updates.
import { callBackend, callBackendVoid } from '../backend/callBackend';
import type { Connection } from '../stores/appState';

export async function fetchConnections(): Promise<Connection[]> {
  // GetAllConnections returns ConnectionDTO[] (backend wire type); Connection
  // is the frontend domain type. They're structurally equivalent at runtime
  // (matches original stores/api.ts, which relied on `app: any`).
  return callBackend('Refresh connections', [], async (app) => (await app.GetAllConnections()) as unknown as Connection[]);
}

export async function putConnection(c: Partial<Connection>): Promise<Connection | null> {
  return callBackend('Save connection', null, async (app) => (await app.SaveConnection(c as unknown as Parameters<typeof app.SaveConnection>[0])) as unknown as Connection);
}

// These three mutations rethrow on failure (unlike most callBackendVoid
// wrappers, which swallow). The still-in-place public functions in
// stores/api.ts (deleteConnection, moveConnections, reorderConnections) need
// to distinguish success from failure to decide whether to refresh the
// `connections` store afterward — matching the original hand-rolled
// try/catch, which wrapped the mutation + refresh in one try block and
// skipped the refresh entirely when the mutation itself failed. The error is
// still reported via showError inside callBackend either way.
export async function deleteConnectionById(id: string): Promise<void> {
  return callBackendVoid('Delete connection', (app) => app.DeleteConnection(id), { rethrow: true });
}

export async function moveConnectionsTo(connectionIds: string[], targetFolderId: string): Promise<void> {
  return callBackendVoid('Move connections', (app) => app.MoveConnections(connectionIds, targetFolderId), { rethrow: true });
}

export async function reorderConnectionsIn(connectionIds: string[], folderId: string): Promise<void> {
  return callBackendVoid('Reorder connections', (app) => app.ReorderConnections(connectionIds, folderId), { rethrow: true });
}
