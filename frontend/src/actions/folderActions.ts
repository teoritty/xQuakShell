// Folder orchestration layer: composes the atomic folder RPCs
// (api/folders.ts) with the `folders` store (and, for deleteFolder, the
// cross-domain `connections` refresh). Moved verbatim from stores/api.ts.
//
// Missing-gateway guard analysis (per-function, matches the original
// stores/api.ts bodies exactly):
// - refreshFolders: original guarded with `getApp()` before ANY store
//   mutation. The atomic fetchFolders silently returns [] on a missing
//   gateway (no throw), so without an explicit guard here `folders.set([])`
//   would run even when the gateway is absent — a regression. The guard is
//   reproduced explicitly below.
// - saveFolder: original had NO explicit guard; it relied on putFolder
//   returning null on a missing gateway (putFolder -> callBackend ->
//   `if (!app) return fallback (null)`), so `if (saved)` already skips the
//   refresh. No guard needed here either.
// - createNewFolderInFolder: original had NO explicit guard; relies on
//   saveFolder (which relies on putFolder) returning null. No guard needed.
// - deleteFolder / deleteFolders / moveFolder / moveFolders / reorderFolders: original
//   guarded with `getApp()` before calling the mutation RPC at all. The
//   atomic deleteFolderById / moveFolderTo / reorderFoldersIn wrappers call
//   `getGateway()` internally and return their fallback *before* the
//   try/catch (so `rethrow` never fires on a missing gateway), meaning
//   without an explicit guard the surrounding try/catch here would proceed
//   straight into the dependent refresh (e.g. `refreshFolders`) even though
//   nothing was mutated — a regression. The guard is reproduced explicitly
//   below for all four. `moveFolders` and `deleteFolders` additionally
//   short-circuit on an empty array. `deleteFolder` is now a one-element
//   `deleteFolders` so the guard exists in exactly one place.
import { getGateway } from '../backend/context';
import {
  fetchFolders,
  putFolder,
  deleteFolderById,
  moveFolderTo,
  reorderFoldersIn,
} from '../api/folders';
import { folders, type Folder } from '../stores/appState';
import { refreshAllConnections } from './connectionActions';

export async function refreshFolders(): Promise<void> {
  if (!getGateway()) return;
  const result = await fetchFolders();
  folders.set(result || []);
}

export async function saveFolder(f: Partial<Folder>): Promise<Folder | null> {
  const saved = await putFolder(f);
  if (saved) {
    await refreshFolders();
  }
  return saved;
}

/**
 * Deliberately touches no selection store. Pointing the creation target at the
 * folder it just made turned every click on "New folder" into a child of the
 * previous one; the target now derives from the tree selection alone
 * (creationTargetFolderId in lib/remoteTree/selection.ts), which also removes
 * the race where the nesting depth depended on how fast the user clicked.
 */
export async function createNewFolderInFolder(parentId: string): Promise<Folder | null> {
  return saveFolder({
    name: 'New folder',
    parentId,
  });
}

export async function deleteFolder(id: string): Promise<void> {
  await deleteFolders([id]);
}

/**
 * One refresh for the whole batch, matching moveFolders: deleting a
 * multi-selection through the single-id function meant a full folder AND
 * connection reload per item.
 */
export async function deleteFolders(ids: string[]): Promise<void> {
  if (!getGateway() || ids.length === 0) return;
  try {
    for (const id of ids) {
      await deleteFolderById(id);
    }
    await refreshFolders();
    await refreshAllConnections();
  } catch {
    // Error already reported by deleteFolderById via showError; skip refresh.
  }
}

export async function moveFolder(folderId: string, targetParentId: string): Promise<void> {
  if (!getGateway()) return;
  try {
    await moveFolderTo(folderId, targetParentId);
    await refreshFolders();
  } catch {
    // Error already reported by moveFolderTo via showError; skip refresh.
  }
}

export async function moveFolders(folderIds: string[], targetParentId: string): Promise<void> {
  if (!getGateway() || folderIds.length === 0) return;
  try {
    for (const folderId of folderIds) {
      await moveFolderTo(folderId, targetParentId, 'Move folders');
    }
    await refreshFolders();
  } catch {
    // Error already reported by moveFolderTo via showError; skip refresh.
  }
}

export async function reorderFolders(folderIds: string[], parentId: string): Promise<void> {
  if (!getGateway()) return;
  try {
    await reorderFoldersIn(folderIds, parentId);
    await refreshFolders();
  } catch {
    // Error already reported by reorderFoldersIn via showError; skip refresh.
  }
}
