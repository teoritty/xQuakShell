import type { Connection, Folder } from '../../stores/appState';
import type { Writable } from 'svelte/store';
import type { TreeNode } from './types';

export interface SelectionStores {
  selectedConnectionId: Writable<string>;
  selectedConnectionIds: Writable<Set<string>>;
  selectedFolderId: Writable<string>;
}

export interface SelectNodeResult {
  selectedPaths: Set<string>;
  lastSelectedPath: string;
}

/** Same rules as FileTree.selectNode — visibleNodes = flat expanded tree rows. */
export function selectTreeNode(
  id: string,
  visibleNodes: TreeNode[],
  lastSelectedPath: string | null,
  selectedPaths: Set<string>,
  e?: MouseEvent
): SelectNodeResult {
  if (e?.ctrlKey || e?.metaKey) {
    const next = new Set(selectedPaths);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    return { selectedPaths: next, lastSelectedPath: id };
  }
  if (e?.shiftKey) {
    const idx = visibleNodes.findIndex((n) => n.id === id);
    const lastIdx = lastSelectedPath != null ? visibleNodes.findIndex((n) => n.id === lastSelectedPath) : -1;
    const next = new Set(selectedPaths);
    const [lo, hi] = lastIdx >= 0 ? (idx < lastIdx ? [idx, lastIdx] : [lastIdx, idx]) : [idx, idx];
    for (let i = lo; i <= hi; i++) next.add(visibleNodes[i].id);
    return { selectedPaths: next, lastSelectedPath: id };
  }
  return { selectedPaths: new Set([id]), lastSelectedPath: id };
}

export function syncSelectionStores(
  selectedPaths: Set<string>,
  connections: Connection[],
  folders: Folder[],
  stores: SelectionStores
): void {
  const connIds = connections.filter((c) => selectedPaths.has(c.id)).map((c) => c.id);
  const folderIds = folders.filter((f) => selectedPaths.has(f.id)).map((f) => f.id);
  stores.selectedConnectionIds.set(new Set(connIds));
  stores.selectedConnectionId.set(connIds.length === 1 ? connIds[0] : '');
  stores.selectedFolderId.set(folderIds.length === 1 ? folderIds[0] : '');
}

export function clearTreeSelection(stores: SelectionStores): Set<string> {
  stores.selectedConnectionId.set('');
  stores.selectedConnectionIds.set(new Set());
  stores.selectedFolderId.set('');
  return new Set();
}

/** Right-click on unselected item → solo select (file-manager style). */
export function prepareContextMenuSelection(
  node: TreeNode,
  selectedPaths: Set<string>
): SelectNodeResult | null {
  if (selectedPaths.has(node.id)) return null;
  return { selectedPaths: new Set([node.id]), lastSelectedPath: node.id };
}

export function connectionIdsInSelection(selectedPaths: Set<string>, connections: Connection[]): string[] {
  return connections.filter((c) => selectedPaths.has(c.id)).map((c) => c.id);
}

export function folderIdsInSelection(selectedPaths: Set<string>, folders: Folder[]): string[] {
  return folders.filter((f) => selectedPaths.has(f.id)).map((f) => f.id);
}

export function connectionIdsForDelete(
  nodeId: string,
  selectedPaths: Set<string>,
  connections: Connection[]
): string[] {
  const connIds = connectionIdsInSelection(selectedPaths, connections);
  if (selectedPaths.has(nodeId) && connIds.length > 1) return connIds;
  return [nodeId];
}
