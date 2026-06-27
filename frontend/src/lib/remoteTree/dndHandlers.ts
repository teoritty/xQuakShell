import { moveConnections, moveFolder, moveFolders, reorderConnections, reorderFolders } from '../../stores/api';
import type { Connection, Folder } from '../../stores/appState';
import {
  allowsReorder,
  isFolderAncestor,
  isNoOpDropOnFolder,
  isNoOpDropOnRoot,
  isNoOpReorder,
  payloadPrimaryId,
  payloadPrimaryType,
} from './dndGuards';
import type { DragPayload, TreeNode } from './types';

function foldersNeedingMove(folderIds: string[], targetFolderId: string, folders: Folder[]): string[] {
  return folderIds.filter((id) => {
    if (id === targetFolderId) return false;
    if (isFolderAncestor(folders, id, targetFolderId)) return false;
    const folder = folders.find((f) => f.id === id);
    return (folder?.parentId || '') !== targetFolderId;
  });
}

function foldersNeedingMoveToRoot(folderIds: string[], folders: Folder[]): string[] {
  return folderIds.filter((id) => {
    const folder = folders.find((f) => f.id === id);
    return !!folder?.parentId;
  });
}

export async function executeDropBetween(
  payload: DragPayload,
  targetNode: TreeNode,
  position: 'before' | 'after',
  flatNodes: TreeNode[],
  connections: Connection[],
  folders: Folder[]
): Promise<void> {
  if (isNoOpReorder(payload, targetNode, flatNodes, position)) return;

  const primaryType = payloadPrimaryType(payload);
  const primaryId = payloadPrimaryId(payload);
  if (!primaryType || !primaryId) return;

  // Resolve mixed-type gaps to the same-type neighbour that forms the gap.
  // e.g. dropping a connection in the gap between the last child of an
  // expanded folder and the next folder below: the target is the folder
  // (different type), so redirect to "after the previous connection".
  let effTarget = targetNode;
  let effPos = position;
  if (targetNode.type !== primaryType) {
    const tIdx = flatNodes.findIndex((n) => n.id === targetNode.id);
    if (position === 'before') {
      const prev = flatNodes[tIdx - 1];
      if (prev && prev.type === primaryType) {
        effTarget = prev;
        effPos = 'after';
      }
    } else {
      const next = flatNodes[tIdx + 1];
      if (next && next.type === primaryType) {
        effTarget = next;
        effPos = 'before';
      }
    }
  }

  const siblings = flatNodes.filter(
    (n) =>
      n.parentId === effTarget.parentId &&
      n.depth === effTarget.depth &&
      n.type === primaryType
  );
  const draggedIdx = siblings.findIndex((n) => n.id === primaryId);
  const targetParentId = effTarget.parentId;

  if (draggedIdx >= 0) {
    const reordered = [...siblings];
    const [removed] = reordered.splice(draggedIdx, 1);
    const newIdx = reordered.findIndex((n) => n.id === effTarget.id);
    const finalIdx = effPos === 'before' ? newIdx : newIdx + 1;
    reordered.splice(finalIdx, 0, removed);
    const ids = reordered.map((n) => n.id);

    if (primaryType === 'connection') {
      await reorderConnections(ids, targetParentId);
    } else {
      await reorderFolders(ids, targetParentId);
    }
    return;
  }

  const ids = siblings.map((n) => n.id);
  const targetIdx = ids.indexOf(effTarget.id);
  if (targetIdx < 0) return;
  const insertIdx = effPos === 'before' ? targetIdx : targetIdx + 1;
  ids.splice(insertIdx, 0, primaryId);

  if (primaryType === 'connection') {
    const conn = connections.find((c) => c.id === primaryId);
    if (conn && (conn.folderId || '') !== targetParentId) {
      await moveConnections([primaryId], targetParentId);
    }
    await reorderConnections(ids, targetParentId);
  } else {
    const folder = folders.find((f) => f.id === primaryId);
    if (folder && (folder.parentId || '') !== targetParentId) {
      await moveFolder(primaryId, targetParentId);
    }
    await reorderFolders(ids, targetParentId);
  }
}

export async function executeDropOnFolder(
  payload: DragPayload,
  targetFolderId: string,
  connections: Connection[],
  folders: Folder[]
): Promise<void> {
  if (isNoOpDropOnFolder(payload, targetFolderId, connections, folders)) return;
  if (payload.connectionIds.length > 0) {
    await moveConnections(payload.connectionIds, targetFolderId);
  }
  const folderIds = foldersNeedingMove(payload.folderIds, targetFolderId, folders);
  if (folderIds.length > 0) {
    await moveFolders(folderIds, targetFolderId);
  }
}

export async function executeDropOnRoot(
  payload: DragPayload,
  connections: Connection[],
  folders: Folder[]
): Promise<void> {
  if (isNoOpDropOnRoot(payload, connections, folders)) return;
  if (payload.connectionIds.length > 0) {
    await moveConnections(payload.connectionIds, '');
  }
  const folderIds = foldersNeedingMoveToRoot(payload.folderIds, folders);
  if (folderIds.length > 0) {
    await moveFolders(folderIds, '');
  }
}

/**
 * Drop on the empty area below the tree: append the dragged item to the end of
 * the root-level list of its own type. Falls back to a plain move-to-root for
 * multi-item drags.
 */
export async function executeDropOnRootEnd(
  payload: DragPayload,
  flatNodes: TreeNode[],
  connections: Connection[],
  folders: Folder[]
): Promise<void> {
  if (!allowsReorder(payload)) {
    await executeDropOnRoot(payload, connections, folders);
    return;
  }

  const primaryType = payloadPrimaryType(payload);
  const primaryId = payloadPrimaryId(payload);
  if (!primaryType || !primaryId) return;

  const rootSiblings = flatNodes.filter(
    (n) => n.parentId === '' && n.depth === 0 && n.type === primaryType
  );
  const ids = rootSiblings.map((n) => n.id).filter((id) => id !== primaryId);
  ids.push(primaryId);

  if (primaryType === 'connection') {
    const conn = connections.find((c) => c.id === primaryId);
    if (conn && (conn.folderId || '') !== '') {
      await moveConnections([primaryId], '');
    }
    await reorderConnections(ids, '');
  } else {
    const folder = folders.find((f) => f.id === primaryId);
    if (folder && (folder.parentId || '') !== '') {
      await moveFolder(primaryId, '');
    }
    await reorderFolders(ids, '');
  }
}

export function parseDragPayload(raw: string): DragPayload | null {
  try {
    const parsed = JSON.parse(raw);
    if (Array.isArray(parsed?.folderIds) || Array.isArray(parsed?.connectionIds)) {
      return {
        folderIds: Array.isArray(parsed.folderIds)
          ? parsed.folderIds.filter((x: unknown) => typeof x === 'string')
          : [],
        connectionIds: Array.isArray(parsed.connectionIds)
          ? parsed.connectionIds.filter((x: unknown) => typeof x === 'string')
          : [],
      };
    }
    if (parsed?.type === 'folder' && typeof parsed.id === 'string') {
      return { folderIds: [parsed.id], connectionIds: [] };
    }
    if (parsed?.type === 'connection') {
      if (Array.isArray(parsed.ids)) {
        return { folderIds: [], connectionIds: parsed.ids.filter((x: unknown) => typeof x === 'string') };
      }
      if (typeof parsed.id === 'string') {
        return { folderIds: [], connectionIds: [parsed.id] };
      }
    }
  } catch {
    return null;
  }
  return null;
}
