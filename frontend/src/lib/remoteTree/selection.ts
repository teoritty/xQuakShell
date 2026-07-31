import type { Connection, Folder } from '../../stores/appState';
import type { Writable } from 'svelte/store';
import { isDiscoveryNodeId, type TreeNode } from './types';

/**
 * Strips discovery ids out of a selection before it is mapped onto real
 * connections or folders.
 *
 * HONEST STATUS: this is a backstop, not a proven layer, and no test can
 * currently falsify it. The functions below already filter the selection through
 * the actual `connections`/`folders` arrays, so an id that is not a real
 * connection id cannot survive them anyway — and DISCOVERY_ID_PREFIX guarantees
 * a discovery id never is one. Replacing this function body with `return
 * selectedPaths` keeps every test green; that was measured, not assumed.
 *
 * It is kept because it stops being redundant the moment the id scheme changes:
 * if discovery rows ever lose their prefix, or an id path appears that does not
 * cross-check against the connections array, this is the guard that is already
 * in place at the last point before ids become deletions. What actually protects
 * the tree today is stated where it is provable — selectTreeNode refusing
 * discovery rows, and the explicit check in connectionIdsForDelete, both of
 * which fail discoveryIsolation.test.ts when removed.
 */
function withoutDiscoveryIds(selectedPaths: Set<string>): Set<string> {
  let hasDiscovery = false;
  for (const id of selectedPaths) {
    if (isDiscoveryNodeId(id)) {
      hasDiscovery = true;
      break;
    }
  }
  if (!hasDiscovery) return selectedPaths;
  return new Set([...selectedPaths].filter((id) => !isDiscoveryNodeId(id)));
}

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
  // A discovery row never enters this selection at all — it has its own
  // (discoverySelection.ts). Returning the selection untouched, rather than
  // clearing it, is the caller's job: RemoteTree clears this set explicitly when
  // it moves the focus to a discovery row.
  if (isDiscoveryNodeId(id)) {
    return { selectedPaths, lastSelectedPath: lastSelectedPath ?? '' };
  }
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
    for (let i = lo; i <= hi; i++) {
      // The range can span an expanded subtree; those rows are skipped rather
      // than ending the range, so Shift still reaches the connection below.
      if (visibleNodes[i].type === 'discovery') continue;
      next.add(visibleNodes[i].id);
    }
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
  const paths = withoutDiscoveryIds(selectedPaths);
  const connIds = connections.filter((c) => paths.has(c.id)).map((c) => c.id);
  const folderIds = folders.filter((f) => paths.has(f.id)).map((f) => f.id);
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
  if (node.type === 'discovery') return null;
  if (selectedPaths.has(node.id)) return null;
  return { selectedPaths: new Set([node.id]), lastSelectedPath: node.id };
}

export function connectionIdsInSelection(selectedPaths: Set<string>, connections: Connection[]): string[] {
  const paths = withoutDiscoveryIds(selectedPaths);
  return connections.filter((c) => paths.has(c.id)).map((c) => c.id);
}

export function folderIdsInSelection(selectedPaths: Set<string>, folders: Folder[]): string[] {
  const paths = withoutDiscoveryIds(selectedPaths);
  return folders.filter((f) => paths.has(f.id)).map((f) => f.id);
}

export function connectionIdsForDelete(
  nodeId: string,
  selectedPaths: Set<string>,
  connections: Connection[]
): string[] {
  // Load-bearing, and provably so: nodeId is returned UNCHECKED on the single-row
  // path below — it is never validated against `connections` — so without this
  // line a delete aimed at a discovery row would hand that row's id straight to
  // deleteConnection. Removing it fails discoveryIsolation.test.ts.
  if (isDiscoveryNodeId(nodeId)) return [];
  const connIds = connectionIdsInSelection(selectedPaths, connections);
  if (selectedPaths.has(nodeId) && connIds.length > 1) return connIds;
  return [nodeId];
}
