/**
 * Which element should be highlighted during an internal (pane-to-pane) file drag.
 *
 * `'row'` means a folder row under the cursor owns the highlight; `'pane'` means the drop will
 * land in the pane's current directory, so the pane itself fills.
 */
export type DragHighlight = 'none' | 'pane' | 'row';

/**
 * Decides the highlight for an internal file drag from the pane's drag state.
 *
 * The file panes set `dragOverPath` to a folder's path when the cursor is over that folder's row,
 * and to their own `currentPath` when it is over a file row or empty space (see
 * `handleDragOverPath` in FileTree.svelte / LocalFileTree.svelte). Comparing the two therefore
 * tells us whether a specific folder is the target or the pane as a whole is.
 *
 * A more specific row wins over the pane, matching `highlightTargetAt` in osFileDrop.ts so that
 * OS drops and internal drops highlight the same way.
 *
 * This exists because the pane fill used to come from the OS-drop router, which only acts on
 * drags carrying real OS files. Internal drags satisfied that gate on WebView2 but not on
 * WebKitGTK, so the fill silently disappeared on Linux. Internal drags now drive their own
 * highlight and no longer depend on engine-specific DataTransfer behaviour.
 */
export function internalDragHighlight(dragOverPath: string | null, currentPath: string): DragHighlight {
  if (dragOverPath === null) return 'none';
  return dragOverPath === currentPath ? 'pane' : 'row';
}
