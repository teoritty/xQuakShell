// Reading the dataTransfer payload a file-pane drag carries.
//
// A drag between panes is described by four keys — a remote path or a JSON
// array of them, and the same pair for local paths — plus the session id that
// says which host the remote paths belong to. Each pane parsed all four inline,
// in its own order, with its own try/catch around JSON.parse. The reads are
// identical; only which side the pane acts on first differs, and that decision
// stays in the pane, where it belongs.
//
// getData is passed as a function rather than a DataTransfer so the parsing is
// testable without a DOM, the same way paneSelection takes a structural node.

export interface DragPayload {
  /** Session the remote paths belong to; empty for a local-only drag. */
  sessionId: string;
  remotePaths: string[];
  localPaths: string[];
}

export type DragDataReader = (key: string) => string;

/** The paths a drag carries, whichever of the single/multi forms it used. */
export function readDragPayload(getData: DragDataReader): DragPayload {
  return {
    sessionId: getData('text/session-id') || '',
    remotePaths: readPathList(getData, 'text/selected-paths', 'text/remote-path'),
    localPaths: readPathList(getData, 'text/local-selected-paths', 'text/local-path'),
  };
}

// A multi-selection is a JSON array; a single row is a bare path. Malformed
// JSON yields no paths rather than throwing: a drag that cannot be understood
// must do nothing, not break the drop handler for the other half of the payload.
function readPathList(getData: DragDataReader, listKey: string, singleKey: string): string[] {
  const json = getData(listKey);
  if (json) {
    try {
      const parsed = JSON.parse(json);
      return Array.isArray(parsed) ? parsed.filter((p): p is string => typeof p === 'string') : [];
    } catch {
      return [];
    }
  }
  const single = getData(singleKey);
  return single ? [single] : [];
}

/**
 * True when dragging `path` should carry the whole selection.
 *
 * Dragging a row that is not part of the current selection moves that row
 * alone — the selection is not silently extended to include what the user
 * actually grabbed.
 */
export function isMultiDrag(selectedPaths: Set<string>, path: string): boolean {
  return selectedPaths.has(path) && selectedPaths.size > 1;
}
