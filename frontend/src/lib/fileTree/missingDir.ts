// What a file pane does when a directory it lists has gone: deleted from this pane, from the
// other one, from a shell, or by another program. Shared by the remote and the local pane.
//
// The directory being shown is replaced by its nearest parent that still exists - the same thing
// a file manager does when the folder it has open is deleted. A directory that was only expanded
// in the tree is dropped from it. Neither case is an error: the user usually caused it on
// purpose, and a "not found" dialog reported their own delete back to them.

/**
 * The backend's domain.ErrDirectoryNotFound, as its message. Wails hands a rejected call over as
 * the message alone, so this string is the contract; the Go side names this file next to it.
 */
export const DIRECTORY_NOT_FOUND = 'directory not found';

export function isDirectoryNotFound(e: unknown): boolean {
  const msg = e instanceof Error ? e.message : String(e ?? '');
  return msg.toLowerCase().includes(DIRECTORY_NOT_FOUND);
}

/**
 * Options for a pane's listing call. Failures are rethrown to the pane, which shows them in its
 * own header, instead of opening the global error dialog; a missing directory is handled here and
 * never reaches either.
 */
export const PANE_LIST_OPTS = { rethrow: true, silence: () => true };

export interface PaneListing<N> {
  /** The directory the pane asked for. */
  requested: string;
  /** The directory actually listed: `requested`, or the nearest existing parent of it. */
  path: string;
  /** null when a directory other than the pane's current one has gone. */
  nodes: N[] | null;
}

/**
 * List `requested`. When it has gone and it is the directory the pane is showing, climb to the
 * nearest parent that still exists; when it is any other directory, report it as gone. Every other
 * failure is rethrown, and so is a missing root - there is nowhere left to climb.
 */
export async function listForPane<N>(
  requested: string,
  currentPath: string,
  list: (path: string) => Promise<N[]>,
  parentOf: (path: string) => string,
): Promise<PaneListing<N>> {
  let path = requested;
  for (;;) {
    try {
      return { requested, path, nodes: await list(path) };
    } catch (e) {
      if (!isDirectoryNotFound(e)) throw e;
      if (requested !== currentPath) return { requested, path, nodes: null };
      const parent = parentOf(path);
      if (!parent || parent === path) throw e;
      path = parent;
    }
  }
}

export interface PaneTreeState<N> {
  tree: Map<string, N[]>;
  rawTree: Map<string, N[]>;
  expanded: Set<string>;
}

/**
 * Store a listing in the pane's maps and return the pane's current path afterwards: unchanged,
 * or the parent the listing climbed to. The maps are mutated in place; the caller reassigns them
 * for Svelte.
 */
export function settleListing<N>(
  state: PaneTreeState<N>,
  listing: PaneListing<N>,
  currentPath: string,
  sort: (nodes: N[]) => N[],
): string {
  if (listing.path !== listing.requested || listing.nodes === null) {
    state.tree.delete(listing.requested);
    state.rawTree.delete(listing.requested);
    state.expanded.delete(listing.requested);
  }
  if (listing.nodes === null) return currentPath;
  state.rawTree.set(listing.path, listing.nodes);
  state.tree.set(listing.path, sort(listing.nodes));
  if (listing.path === listing.requested) return currentPath;
  state.expanded.add(listing.path);
  return listing.path;
}
