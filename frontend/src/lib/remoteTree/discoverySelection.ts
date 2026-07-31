// Selection for discovery rows — deliberately NOT selection.ts.
//
// The two sets are kept apart on purpose and never merge. A discovery node is a
// remote resource that a plugin drew; a connection is local config the user can
// rename, move and delete. If a discovery row could ever land in the connection
// selection, a Shift-drag across an expanded subtree would reach the tree's
// "Delete" item and destroy connections the user never pointed at. That is the
// single worst failure mode in this feature, so the two selections are separate
// values with separate types and selection.ts additionally filters by id prefix.
//
// Invariants enforced here:
//  - every selected row is a child of ONE parent (parentKey is plugin-scoped, so
//    that also means one plugin — which is what makes the selection addressable
//    by the single pluginId InvokeDiscoveryAction requires);
//  - Shift/Ctrl never widen the selection past that parent;
//  - a row that disappeared from the snapshot drops out of the selection.
import type { DiscoveryRow } from './types';

export interface DiscoverySelection {
  /** '' when nothing is selected. */
  connectionId: string;
  pluginId: string;
  parentKey: string;
  /** discoveryKey values of the selected rows. */
  keys: Set<string>;
  lastKey: string;
}

export function emptyDiscoverySelection(): DiscoverySelection {
  return { connectionId: '', pluginId: '', parentKey: '', keys: new Set(), lastKey: '' };
}

export function isDiscoverySelectionEmpty(selection: DiscoverySelection): boolean {
  return selection.keys.size === 0;
}

function soloSelect(row: DiscoveryRow): DiscoverySelection {
  return {
    connectionId: row.connectionId,
    pluginId: row.pluginId,
    parentKey: row.parentKey,
    keys: new Set([row.key]),
    lastKey: row.key,
  };
}

/** Rows sharing a parent with `row`, in the visible order of `visibleRows`. */
function siblingsOf(row: DiscoveryRow, visibleRows: DiscoveryRow[]): DiscoveryRow[] {
  return visibleRows.filter(
    (r) => r.connectionId === row.connectionId && r.parentKey === row.parentKey
  );
}

/**
 * Ctrl/Cmd toggles, Shift extends, a plain click replaces — the same feel as the
 * connection tree, except that both modifiers collapse to a plain click the
 * moment they would cross a parent boundary. Silently dropping the modifier is
 * better than silently selecting rows under a different parent: the user can see
 * what got selected, and no action can then be aimed at a set the backend cannot
 * address.
 */
export function selectDiscoveryRow(
  current: DiscoverySelection,
  row: DiscoveryRow,
  visibleRows: DiscoveryRow[],
  e?: { ctrlKey?: boolean; metaKey?: boolean; shiftKey?: boolean }
): DiscoverySelection {
  const sameParent =
    current.keys.size > 0 &&
    current.connectionId === row.connectionId &&
    current.parentKey === row.parentKey;

  if ((e?.ctrlKey || e?.metaKey) && sameParent) {
    const keys = new Set(current.keys);
    if (keys.has(row.key)) keys.delete(row.key);
    else keys.add(row.key);
    if (keys.size === 0) return emptyDiscoverySelection();
    return { ...current, keys, lastKey: row.key };
  }

  if (e?.shiftKey && sameParent) {
    const siblings = siblingsOf(row, visibleRows);
    const idx = siblings.findIndex((r) => r.key === row.key);
    const lastIdx = siblings.findIndex((r) => r.key === current.lastKey);
    if (idx < 0) return soloSelect(row);
    const [lo, hi] = lastIdx >= 0 ? (idx < lastIdx ? [idx, lastIdx] : [lastIdx, idx]) : [idx, idx];
    const keys = new Set(current.keys);
    for (let i = lo; i <= hi; i++) keys.add(siblings[i].key);
    return { ...current, keys, lastKey: row.key };
  }

  return soloSelect(row);
}

/**
 * Drops selected rows that are no longer in the snapshot. Discovery is a level,
 * not an edge: a republish can remove a container that no longer exists, and a
 * menu still aimed at it would invoke an action on a node id the plugin has
 * forgotten. An emptied selection returns to the empty value, which is the
 * signal the caller uses to close the action menu.
 */
export function pruneDiscoverySelection(
  current: DiscoverySelection,
  visibleRows: DiscoveryRow[]
): DiscoverySelection {
  if (current.keys.size === 0) return current;
  const alive = new Set(
    visibleRows.filter((r) => r.connectionId === current.connectionId).map((r) => r.key)
  );
  const keys = new Set([...current.keys].filter((k) => alive.has(k)));
  if (keys.size === 0) return emptyDiscoverySelection();
  if (keys.size === current.keys.size) return current;
  return {
    ...current,
    keys,
    lastKey: keys.has(current.lastKey) ? current.lastKey : [...keys][keys.size - 1],
  };
}

/**
 * Moves the selection one row up or down.
 *
 * Plain arrows walk every visible discovery row of the connection, so the cursor
 * can step into and back out of an expanded group the same way it steps over any
 * other row. Shift only ever reaches siblings: extending across a parent would
 * build a set no single invokeAction could address.
 */
export function moveDiscoverySelection(
  current: DiscoverySelection,
  visibleRows: DiscoveryRow[],
  direction: -1 | 1,
  extend: boolean
): DiscoverySelection {
  if (current.keys.size === 0) return current;
  const scope = extend
    ? visibleRows.filter(
        (r) => r.connectionId === current.connectionId && r.parentKey === current.parentKey
      )
    : visibleRows.filter((r) => r.connectionId === current.connectionId);
  const fromIdx = scope.findIndex((r) => r.key === current.lastKey);
  if (fromIdx < 0) return current;
  const next = scope[fromIdx + direction];
  if (!next) return current;
  return selectDiscoveryRow(current, next, visibleRows, extend ? { shiftKey: true } : undefined);
}

/** The selected rows, in visible order — what the action menu is computed from. */
export function selectedDiscoveryRows(
  selection: DiscoverySelection,
  visibleRows: DiscoveryRow[]
): DiscoveryRow[] {
  if (selection.keys.size === 0) return [];
  return visibleRows.filter(
    (r) => r.connectionId === selection.connectionId && selection.keys.has(r.key)
  );
}
