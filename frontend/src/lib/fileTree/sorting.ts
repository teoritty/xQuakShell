// Listing order and size formatting for the file panes.
//
// Both panes sorted directories first, then by the active column, then by name
// as a tiebreak — in two copies of the same code, one of which had picked up a
// null guard the other never got. The order is a property of a listing, not of
// a pane, so it lives here once and takes its sort state as an argument rather
// than reading a component's variables.

import type { SortKey } from '../filePanelToolbar';
import type { FileNode, FileTreeMap, SortState } from './types';

/** Human-readable file size. Binary steps, decimal-style labels, as the panes have always shown them. */
export function formatSize(size: number): string {
  if (size < 1024) return `${size} B`;
  if (size < 1048576) return `${(size / 1024).toFixed(1)} KB`;
  if (size < 1073741824) return `${(size / 1048576).toFixed(1)} MB`;
  return `${(size / 1073741824).toFixed(1)} GB`;
}

/** Milliseconds since the epoch, or -1 when the timestamp is missing or unparseable, so it sorts oldest. */
export function parseTimestamp(value?: string): number {
  if (!value) return -1;
  const ts = Date.parse(value);
  return Number.isFinite(ts) ? ts : -1;
}

export function compareValues(a: number | string, b: number | string): number {
  if (typeof a === 'string' && typeof b === 'string') return a.localeCompare(b);
  return Number(a) - Number(b);
}

/**
 * The value a node sorts by under `key`. The owner column falls back to the
 * group when there is no owner, which matters for remote listings and is simply
 * inert for local ones (they carry no group).
 */
export function sortValue(node: FileNode, key: SortKey): number | string {
  if (key === 'name') return node.name.toLowerCase();
  if (key === 'size') return node.size ?? 0;
  if (key === 'modTime') return parseTimestamp(node.modTime);
  return (node.owner || node.group || '').toLowerCase();
}

/**
 * Sort one directory listing. Directories always come first — that is the pane's
 * shape, not a sort option — and name is the final tiebreak so the order is
 * total and a re-sort never reshuffles equal rows.
 *
 * Returns a new array; the input is left alone.
 */
export function applySort<T extends FileNode>(nodes: T[], state: SortState): T[] {
  if (!nodes) return [];
  const dir = state.sortDir === 'asc' ? 1 : -1;
  const key = state.sortKey;
  return [...nodes].sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
    if (state.sortEnabled && key) {
      const cmp = compareValues(sortValue(a, key), sortValue(b, key));
      if (cmp !== 0) return cmp * dir;
    }
    return a.name.localeCompare(b.name);
  });
}

/**
 * Re-sort every listing a pane holds. With sorting off this is the raw tree
 * copied, which is what restores the server's own order when the user turns
 * sorting off again.
 */
export function sortTree<T extends FileNode>(rawTree: FileTreeMap<T>, state: SortState): FileTreeMap<T> {
  if (!state.sortEnabled || !state.sortKey) return new Map(rawTree);
  const next: FileTreeMap<T> = new Map();
  for (const [path, nodes] of rawTree.entries()) {
    next.set(path, applySort(nodes, state));
  }
  return next;
}
