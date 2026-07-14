// Connection (and identity) orchestration layer: composes the atomic
// connection/identity RPCs (api/connections.ts, api/credentials.ts) with the
// `connections`/`identities` stores. Moved verbatim from stores/api.ts.
//
// Missing-gateway guard analysis (per-function, matches the original
// stores/api.ts bodies exactly):
// - refreshAllConnections / refreshIdentities: original guarded with
//   `getApp()` before ANY store mutation. The atomic fetchConnections /
//   fetchIdentities silently return [] on a missing gateway (no throw), so
//   without an explicit guard here `connections.set([])` /
//   `identities.set([])` would run even when the gateway is absent — a
//   regression. The guard is reproduced explicitly below.
// - saveConnection: original had NO explicit guard; it relied on
//   putConnection returning null on a missing gateway (putConnection ->
//   callBackend -> `if (!app) return fallback (null)`), so `if (saved)`
//   already skips the refresh. No guard needed here either.
// - createNewConnectionInFolder: original had NO explicit guard; relies on
//   saveConnection (which relies on putConnection) returning null. No guard
//   needed.
// - deleteConnection / moveConnections / reorderConnections: original
//   guarded with `getApp()` before calling the mutation RPC at all. The
//   atomic deleteConnectionById / moveConnectionsTo / reorderConnectionsIn
//   wrappers call `getGateway()` internally and return their fallback
//   *before* the try/catch (so `rethrow` never fires on a missing gateway),
//   meaning without an explicit guard the surrounding try/catch here would
//   proceed straight into the dependent refresh (e.g. `refreshAllConnections`)
//   even though nothing was mutated — a regression. The guard is reproduced
//   explicitly below for all three.
import { getGateway } from '../backend/context';
import {
  fetchConnections,
  putConnection,
  deleteConnectionById,
  moveConnectionsTo,
  reorderConnectionsIn,
} from '../api/connections';
import { fetchIdentities } from '../api/credentials';
import {
  connections, identities,
  selectedConnectionId, detailsConnectionId,
  type Connection,
} from '../stores/appState';

export async function refreshAllConnections(): Promise<void> {
  if (!getGateway()) return;
  const result = await fetchConnections();
  connections.set(result || []);
}

export async function refreshIdentities(): Promise<void> {
  if (!getGateway()) return;
  const result = await fetchIdentities();
  identities.set(result || []);
}

export async function saveConnection(c: Partial<Connection>): Promise<Connection | null> {
  const saved = await putConnection(c);
  if (saved) {
    await refreshAllConnections();
  }
  return saved;
}

export async function createNewConnectionInFolder(folderId: string): Promise<Connection | null> {
  const uid = `u-${Date.now()}-${Math.random().toString(36).slice(2, 9)}`;
  const saved = await saveConnection({
    name: 'New connection',
    host: '',
    port: 22,
    folderId,
    users: [{ id: uid, username: '', authMethod: 'key' }],
    defaultUserId: uid,
  });
  if (saved) {
    selectedConnectionId.set(saved.id);
    detailsConnectionId.set(saved.id);
  }
  return saved;
}

export async function deleteConnection(id: string): Promise<void> {
  if (!getGateway()) return;
  try {
    await deleteConnectionById(id);
    await refreshAllConnections();
  } catch {
    // Error already reported by deleteConnectionById via showError; skip refresh.
  }
}

export async function moveConnections(connectionIds: string[], targetFolderId: string): Promise<void> {
  if (!getGateway()) return;
  try {
    await moveConnectionsTo(connectionIds, targetFolderId);
    await refreshAllConnections();
  } catch {
    // Error already reported by moveConnectionsTo via showError; skip refresh.
  }
}

export async function reorderConnections(connectionIds: string[], folderId: string): Promise<void> {
  if (!getGateway()) return;
  try {
    await reorderConnectionsIn(connectionIds, folderId);
    await refreshAllConnections();
  } catch {
    // Error already reported by reorderConnectionsIn via showError; skip refresh.
  }
}
