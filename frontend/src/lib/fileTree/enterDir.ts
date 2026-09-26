// Moving a file pane into another directory. Shared by the remote and the local pane.
//
// The pane's path changes only after the target has been listed. Committing the path first and
// listing second is what left a pane "inside" /root with an empty listing and a permission-denied
// message: the header named a folder the user could not open, and every later action - refresh,
// new folder, upload - aimed at it. A directory that cannot be listed, for any reason, leaves the
// pane exactly where it was.
//
// Only the latest request counts. Opening a slow directory and then a fast one must end in the
// second; without the turn check the slow listing lands last and pulls the pane back to it.

import type { PaneTreeState } from './missingDir';

export type EnterResult = 'entered' | 'superseded';

export type EnterDirectory<N> = (state: PaneTreeState<N>, target: string) => Promise<EnterResult>;

/**
 * Build a pane's "enter this directory" step over its listing call.
 *
 * On 'entered' the listing is stored in the pane's maps and the target is marked expanded; the
 * caller then makes it the current path. A failed listing is rethrown with the maps untouched. A
 * request overtaken by a newer one resolves 'superseded' - even when it failed - and changes
 * nothing, so its error is never shown against the directory the pane has since moved to.
 */
export function createDirectoryEntry<N>(
  list: (path: string) => Promise<N[]>,
  sort: (nodes: N[]) => N[],
): EnterDirectory<N> {
  let turn = 0;
  return async (state, target) => {
    const mine = ++turn;
    let nodes: N[];
    try {
      nodes = await list(target);
    } catch (e) {
      if (mine !== turn) return 'superseded';
      throw e;
    }
    if (mine !== turn) return 'superseded';
    state.rawTree.set(target, nodes);
    state.tree.set(target, sort(nodes));
    state.expanded.add(target);
    return 'entered';
  };
}
