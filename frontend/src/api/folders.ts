// Atomic folder RPC wrappers. Each function is a thin wrapper around a
// single backend RPC call, routed through callBackend/callBackendVoid for
// uniform error handling. No store access here — orchestration (Phase 3)
// combines these with `folders` store updates.
import { callBackend, callBackendVoid } from '../backend/callBackend';
import type { Folder } from '../stores/appState';

export async function fetchFolders(): Promise<Folder[]> {
  return callBackend('Refresh folders', [], (app) => app.GetFolders());
}

export async function putFolder(f: Partial<Folder>): Promise<Folder | null> {
  return callBackend('Save folder', null, (app) => app.SaveFolder(f as Folder));
}

// These three mutations rethrow on failure (unlike most callBackendVoid
// wrappers, which swallow). The still-in-place public functions in
// stores/api.ts (deleteFolder, moveFolder, moveFolders, reorderFolders) need
// to distinguish success from failure to decide whether to refresh the
// `folders`/`connections` stores afterward — matching the original
// hand-rolled try/catch, which wrapped the mutation + refresh in one try
// block and skipped the refresh entirely when the mutation itself failed.
// The error is still reported via showError inside callBackend either way.
export async function deleteFolderById(id: string): Promise<void> {
  return callBackendVoid('Delete folder', (app) => app.DeleteFolder(id), { rethrow: true });
}

// `context` defaults to 'Move folder' for single-folder callers. The bulk
// caller (moveFolders in stores/api.ts) passes 'Move folders' so a failure
// mid-loop reports with the original plural context instead of the atomic
// wrapper's singular one.
export async function moveFolderTo(folderId: string, targetParentId: string, context = 'Move folder'): Promise<void> {
  return callBackendVoid(context, (app) => app.MoveFolder(folderId, targetParentId), { rethrow: true });
}

export async function reorderFoldersIn(folderIds: string[], parentId: string): Promise<void> {
  return callBackendVoid('Reorder folders', (app) => app.ReorderFolders(folderIds, parentId), { rethrow: true });
}
