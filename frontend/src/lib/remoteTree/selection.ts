import type { Connection, Folder } from '../../stores/appState';
import type { Writable } from 'svelte/store';
import { isFolderAncestor } from './dndGuards';
import { isDiscoveryNodeId, type TreeNode } from './types';

/**
 * Strips discovery ids out of a selection before it is mapped onto real
 * connections or folders — and complains loudly when it has to.
 *
 * Reaching the `if` below means an invariant broke upstream: a discovery row got
 * into the connection selection, which selectTreeNode is supposed to make
 * impossible. Filtering silently would leave that regression to be discovered
 * later, by someone losing connections; warning turns an unfalsifiable backstop
 * into a detector that says which id and which layer failed.
 *
 * On its own the filtering is redundant today — the callers already intersect
 * the selection with the real `connections`/`folders` arrays, and
 * DISCOVERY_ID_PREFIX guarantees a discovery id is in neither. That was measured:
 * replacing the body with `return selectedPaths` kept every test green. It stops
 * being redundant if the id scheme changes, and the warning is what makes it
 * testable in the meantime.
 */
function withoutDiscoveryIds(selectedPaths: Set<string>): Set<string> {
  const offenders: string[] = [];
  for (const id of selectedPaths) {
    if (isDiscoveryNodeId(id)) offenders.push(id);
  }
  if (offenders.length === 0) return selectedPaths;
  console.warn(
    `[remoteTree] ${offenders.length} discovery node id(s) reached the connection selection and were ` +
      `dropped. This is a bug upstream — discovery rows have their own selection and selectTreeNode ` +
      `must refuse them. Offending ids: ${offenders.join(', ')}`
  );
  return new Set([...selectedPaths].filter((id) => !isDiscoveryNodeId(id)));
}

export interface SelectionStores {
  selectedConnectionId: Writable<string>;
  selectedConnectionIds: Writable<Set<string>>;
  creationTargetFolderId: Writable<string>;
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

/**
 * The single place that answers "where does a new folder or connection go?".
 *
 * A selected folder means "inside it". A selected connection means "next to
 * it" — i.e. into the folder that holds it, which is what the user sees: the
 * highlighted row lives three folders deep, so the new sibling belongs there
 * too, not at the root. Anything ambiguous (nothing selected, folders and
 * connections at once, connections spread over several folders) has no single
 * honest answer, so it falls back to the root rather than guessing one of them.
 *
 * Derived from `selectedPaths` alone. That is the whole point: the target is a
 * function of what is highlighted, so no action can move it as a side effect.
 */
export function creationTargetFolderId(
  selectedPaths: Set<string>,
  connections: Connection[],
  folders: Folder[]
): string {
  const folderIds = folderIdsInSelection(selectedPaths, folders);
  const connIds = connectionIdsInSelection(selectedPaths, connections);
  if (folderIds.length === 1 && connIds.length === 0) return folderIds[0];
  if (folderIds.length === 0 && connIds.length > 0) {
    const selected = new Set(connIds);
    const parents = new Set(
      connections.filter((c) => selected.has(c.id)).map((c) => c.folderId || '')
    );
    if (parents.size === 1) return [...parents][0];
  }
  return '';
}

export function syncSelectionStores(
  selectedPaths: Set<string>,
  connections: Connection[],
  folders: Folder[],
  stores: SelectionStores
): void {
  const paths = withoutDiscoveryIds(selectedPaths);
  const connIds = connections.filter((c) => paths.has(c.id)).map((c) => c.id);
  stores.selectedConnectionIds.set(new Set(connIds));
  stores.selectedConnectionId.set(connIds.length === 1 ? connIds[0] : '');
  stores.creationTargetFolderId.set(creationTargetFolderId(paths, connections, folders));
}

export function clearTreeSelection(stores: SelectionStores): Set<string> {
  stores.selectedConnectionId.set('');
  stores.selectedConnectionIds.set(new Set());
  stores.creationTargetFolderId.set('');
  return new Set();
}

/**
 * Whether a click that reached the window should drop the tree selection.
 *
 * Only genuinely empty space counts. The toolbar is chrome of the tree, not
 * "outside" it: its buttons are commands that act ON the selection, so letting
 * their own click clear it made "New connection" land in the selected folder
 * the first time and in the root the second. Expressed as one predicate rather
 * than a `stopPropagation` on each button, so a button added later cannot
 * reintroduce that by forgetting the modifier.
 */
export function shouldClearTreeSelection(target: Element | null): boolean {
  if (!target) return false;
  return !target.closest('.tree-node, .context-menu, .tree-toolbar, .import-menu');
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

export interface DeleteTargets {
  folderIds: string[];
  connectionIds: string[];
}

export function deleteTargetCount(targets: DeleteTargets): number {
  return targets.folderIds.length + targets.connectionIds.length;
}

/**
 * What a delete aimed at `nodeId` actually removes.
 *
 * Folders and connections are resolved by ONE rule, deliberately: they used to
 * have two, and the folder half simply ignored the selection, so selecting five
 * folders and hitting Delete removed one of them. Whatever the delete verb is
 * (context menu, row button, Delete key), it acts on the same set.
 *
 * The rule is the file-manager one: acting on a row that is part of the
 * selection acts on the whole selection; acting on a row outside it acts on
 * that row alone (right-click already solo-selects it, see
 * prepareContextMenuSelection).
 */
export function deleteTargets(
  nodeId: string,
  selectedPaths: Set<string>,
  connections: Connection[],
  folders: Folder[]
): DeleteTargets {
  // Load-bearing, and provably so: nodeId is used UNCHECKED on the single-row
  // path below — it is never validated against `connections`/`folders` — so
  // without this line a delete aimed at a discovery row would hand that row's
  // id straight to the delete RPCs. Removing it fails discoveryIsolation.test.ts.
  if (isDiscoveryNodeId(nodeId)) return { folderIds: [], connectionIds: [] };
  const selectionSize = withoutDiscoveryIds(selectedPaths).size;
  if (selectedPaths.has(nodeId) && selectionSize > 1) {
    return selectionDeleteTargets(selectedPaths, connections, folders);
  }
  if (folders.some((f) => f.id === nodeId)) return { folderIds: [nodeId], connectionIds: [] };
  return { folderIds: [], connectionIds: [nodeId] };
}

/**
 * Everything the current selection would delete. Used by the Delete key, which
 * has no clicked row to aim at — it acts on the selection as a whole, and it
 * covers folders for the same reason the menu does.
 */
export function selectionDeleteTargets(
  selectedPaths: Set<string>,
  connections: Connection[],
  folders: Folder[]
): DeleteTargets {
  return collapseCascadedTargets(
    {
      folderIds: folderIdsInSelection(selectedPaths, folders),
      connectionIds: connectionIdsInSelection(selectedPaths, connections),
    },
    connections,
    folders
  );
}

/**
 * Drops everything the backend is going to delete anyway.
 *
 * DeleteFolder removes the folder's whole subtree — nested folders and every
 * connection in them (internal/infra/persistence/connection_repo.go). So a
 * selection spanning a folder and something inside it must not ask twice: the
 * second call would name an id that no longer exists and surface as an error
 * on a delete that in fact succeeded.
 */
function collapseCascadedTargets(
  targets: DeleteTargets,
  connections: Connection[],
  folders: Folder[]
): DeleteTargets {
  const roots = targets.folderIds.filter(
    (id) => !targets.folderIds.some((other) => other !== id && isFolderAncestor(folders, other, id))
  );
  // isFolderAncestor treats a folder as an ancestor of itself, so this covers
  // both "the connection sits directly in a doomed folder" and "somewhere below
  // it" in one test.
  const connectionIds = targets.connectionIds.filter((id) => {
    const parentId = connections.find((c) => c.id === id)?.folderId || '';
    if (!parentId) return true;
    return !roots.some((rootId) => isFolderAncestor(folders, rootId, parentId));
  });
  return { folderIds: roots, connectionIds };
}
