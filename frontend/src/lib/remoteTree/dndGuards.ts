import type { Connection, Folder } from '../../stores/appState';
import type { DragPayload, DropZone, TreeNode } from './types';

export function isNodeEditing(
  node: TreeNode,
  editingFolderId: string | null,
  editingConnId: string | null
): boolean {
  if (node.type === 'folder') return editingFolderId === node.id;
  return editingConnId === node.id;
}

export function resolveDragPayload(
  node: TreeNode,
  selectedPaths: Set<string>,
  selectedFolderIds: Set<string>,
  selectedConnectionIds: Set<string>
): DragPayload {
  const multiSelection =
    selectedPaths.has(node.id) && selectedFolderIds.size + selectedConnectionIds.size > 1;
  if (multiSelection) {
    return {
      folderIds: [...selectedFolderIds],
      connectionIds: [...selectedConnectionIds],
    };
  }
  if (node.type === 'folder') {
    return { folderIds: [node.id], connectionIds: [] };
  }
  return { folderIds: [], connectionIds: [node.id] };
}

export function payloadItemCount(payload: DragPayload): number {
  return payload.folderIds.length + payload.connectionIds.length;
}

export function payloadPrimaryId(payload: DragPayload): string | null {
  if (payload.folderIds.length === 1 && payload.connectionIds.length === 0) return payload.folderIds[0];
  if (payload.connectionIds.length === 1 && payload.folderIds.length === 0) return payload.connectionIds[0];
  return null;
}

export function payloadPrimaryType(payload: DragPayload): 'folder' | 'connection' | null {
  if (payload.folderIds.length === 1 && payload.connectionIds.length === 0) return 'folder';
  if (payload.connectionIds.length === 1 && payload.folderIds.length === 0) return 'connection';
  return null;
}

export function allowsReorder(payload: DragPayload): boolean {
  return payloadItemCount(payload) === 1;
}

export function isFolderAncestor(folders: Folder[], ancestorId: string, candidateParentId: string): boolean {
  if (!candidateParentId) return false;
  let current: string | undefined = candidateParentId;
  while (current) {
    if (current === ancestorId) return true;
    current = folders.find((f) => f.id === current)?.parentId;
  }
  return false;
}

export function allConnectionsInFolder(ids: string[], connections: Connection[], folderId: string): boolean {
  const set = new Set(ids);
  for (const c of connections) {
    if (set.has(c.id) && (c.folderId || '') !== folderId) return false;
  }
  return true;
}

function folderNeedsMoveOnFolder(
  folderId: string,
  targetFolderId: string,
  folders: Folder[]
): boolean {
  if (folderId === targetFolderId) return false;
  if (isFolderAncestor(folders, folderId, targetFolderId)) return false;
  const folder = folders.find((f) => f.id === folderId);
  if (folder && (folder.parentId || '') === targetFolderId) return false;
  return true;
}

export function isNoOpDropOnFolder(
  payload: DragPayload,
  targetFolderId: string,
  connections: Connection[],
  folders: Folder[]
): boolean {
  const folderNeedsMove = payload.folderIds.some((id) =>
    folderNeedsMoveOnFolder(id, targetFolderId, folders)
  );
  const connNeedsMove = !allConnectionsInFolder(payload.connectionIds, connections, targetFolderId);
  return !folderNeedsMove && !connNeedsMove;
}

export function isNoOpDropOnRoot(payload: DragPayload, connections: Connection[], folders: Folder[]): boolean {
  const folderNeedsMove = payload.folderIds.some((id) => {
    const folder = folders.find((f) => f.id === id);
    return !!folder?.parentId;
  });
  const connNeedsMove = !allConnectionsInFolder(payload.connectionIds, connections, '');
  return !folderNeedsMove && !connNeedsMove;
}

export function isNoOpReorder(
  payload: DragPayload,
  targetNode: TreeNode,
  flatNodes: TreeNode[],
  position: 'before' | 'after'
): boolean {
  const primaryType = payloadPrimaryType(payload);
  const primaryId = payloadPrimaryId(payload);
  if (!primaryType || !primaryId) return true;
  if (primaryType === 'folder' && primaryId === targetNode.id) return true;
  if (primaryType === 'connection' && primaryId === targetNode.id) return true;
  if (!allowsReorder(payload)) return true;

  const siblings = flatNodes.filter(
    (n) =>
      n.parentId === targetNode.parentId &&
      n.depth === targetNode.depth &&
      n.type === primaryType
  );
  const draggedIdx = siblings.findIndex((n) => n.id === primaryId);
  if (draggedIdx < 0) return false;

  const reordered = [...siblings];
  const [removed] = reordered.splice(draggedIdx, 1);
  const newIdx = reordered.findIndex((n) => n.id === targetNode.id);
  const finalIdx = position === 'before' ? newIdx : newIdx + 1;
  reordered.splice(finalIdx, 0, removed);
  const newOrder = reordered.map((n) => n.id);
  const oldOrder = siblings.map((n) => n.id);
  return newOrder.join('\0') === oldOrder.join('\0');
}

export function isNoOpDragOver(payload: DragPayload, node: TreeNode): boolean {
  if (payload.folderIds.includes(node.id)) return true;
  if (payload.connectionIds.length === 1 && payload.connectionIds[0] === node.id && payload.folderIds.length === 0) {
    return true;
  }
  return false;
}

export function computeDropZone(e: DragEvent, node: TreeNode): DropZone {
  const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
  const y = e.clientY - rect.top;
  const ratio = y / rect.height;
  if (node.type === 'folder') {
    if (ratio < 0.25) return 'before';
    if (ratio > 0.75) return 'after';
    return 'folder';
  }
  return ratio < 0.5 ? 'before' : 'after';
}

export function shouldShowDropIndicator(
  payload: DragPayload,
  node: TreeNode,
  zone: DropZone,
  connections: Connection[],
  folders: Folder[],
  flatNodes: TreeNode[]
): boolean {
  if (isNoOpDragOver(payload, node)) return false;
  if (zone === 'folder' && node.type === 'folder') {
    return !isNoOpDropOnFolder(payload, node.id, connections, folders);
  }
  if (zone === 'before' || zone === 'after') {
    if (!allowsReorder(payload)) return false;
    return !isNoOpReorder(payload, node, flatNodes, zone);
  }
  return true;
}
