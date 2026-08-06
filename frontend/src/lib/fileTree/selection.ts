// Row selection algebra for the file panes.
//
// This was the same twenty lines in both panes, character for character: ctrl
// or meta toggles one row, shift extends from the last anchor, a plain click
// replaces the selection. It is pure set arithmetic over a listing, so it does
// not need a component — and once it is out here, the shift-range edge cases
// (no anchor yet, anchor scrolled out of the current directory, backwards
// range) can actually be tested.

import type { FileNode } from './types';

/** The pointer modifiers a row click carries. Structural so a test needs no MouseEvent. */
export interface SelectionModifiers {
  ctrlKey?: boolean;
  metaKey?: boolean;
  shiftKey?: boolean;
}

export interface Selection {
  selectedPaths: Set<string>;
  lastSelectedPath: string | null;
}

/**
 * The selection after clicking `path` in `nodes`.
 *
 * Shift-extend deliberately leaves the anchor where it was: holding shift and
 * clicking again re-extends from the original anchor rather than walking it
 * along, which is what every file manager does. A shift-click with no usable
 * anchor selects just the clicked row.
 */
export function selectNode<T extends FileNode>(
  nodes: T[],
  current: Selection,
  path: string,
  modifiers?: SelectionModifiers,
): Selection {
  if (modifiers?.ctrlKey || modifiers?.metaKey) {
    const next = new Set(current.selectedPaths);
    if (next.has(path)) next.delete(path);
    else next.add(path);
    return { selectedPaths: next, lastSelectedPath: path };
  }
  if (modifiers?.shiftKey) {
    return {
      selectedPaths: extendRange(nodes, current, path),
      lastSelectedPath: current.lastSelectedPath,
    };
  }
  return { selectedPaths: new Set([path]), lastSelectedPath: path };
}

function extendRange<T extends FileNode>(nodes: T[], current: Selection, path: string): Set<string> {
  const idx = nodes.findIndex((n) => n.path === path);
  const anchor = current.lastSelectedPath;
  const lastIdx = anchor != null ? nodes.findIndex((n) => n.path === anchor) : -1;
  const next = new Set(current.selectedPaths);
  if (idx < 0) return next;
  const [lo, hi] = lastIdx >= 0 ? (idx < lastIdx ? [idx, lastIdx] : [lastIdx, idx]) : [idx, idx];
  for (let i = lo; i <= hi; i++) next.add(nodes[i].path);
  return next;
}

/** The empty selection. */
export function clearSelection(): Selection {
  return { selectedPaths: new Set(), lastSelectedPath: null };
}

/** Find a node by path across every directory a pane has loaded. */
export function findNode<T extends FileNode>(tree: Map<string, T[]>, path: string): T | undefined {
  for (const [, nodes] of tree) {
    const n = nodes.find((x) => x.path === path);
    if (n) return n;
  }
  return undefined;
}
