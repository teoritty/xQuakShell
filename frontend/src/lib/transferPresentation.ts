// Presentation-layer decisions for the Transfers panel, extracted from
// TransferPanel.svelte so they are unit-testable without a component harness.
//
// The backend reports honest operation facts (kind, state, done/total); how
// those facts are labelled, whether a byte-rate applies, and whether an
// operation counts as "scanning" are display decisions that belong here, not
// in the usecase layer (see the removal of batchDisplayKind).
import type { OperationKind } from '../stores/appState';

const KIND_LABEL: Record<OperationKind, string> = {
  upload: 'Upload',
  download: 'Download',
  localcopy: 'Copy',
  delete: 'Delete',
  chmod: 'chmod',
  chown: 'chown',
};

/** Human-readable label for an operation kind, used in the panel row title
 *  and the completion notification. */
export function kindLabel(kind: OperationKind): string {
  return KIND_LABEL[kind] ?? 'Operation';
}

/** Whether an active item of this kind moves bytes and so has a meaningful
 *  transfer rate. Local copies move bytes just like uploads/downloads. */
export function showsRate(kind: OperationKind, state: string): boolean {
  return (kind === 'upload' || kind === 'download' || kind === 'localcopy') && state === 'active';
}

/** An operation is "scanning" — enumerating its sources with no known total
 *  yet — when it is active and its total is not yet known. This holds
 *  regardless of kind, so adding a new kind never requires touching this
 *  predicate. */
export function isScanning(state: string, total: number): boolean {
  return state === 'active' && total <= 0;
}

// ---------------------------------------------------------------------------
// Which pane reloads what when an operation finishes.
//
// The backend answers "which directory" in `refreshDir`, which is always a real
// path. `remotePath` is a caption ("3 items") and is never parsed here — the
// old regex that derived a directory from it is gone, along with the nonsense
// paths it produced for batches.
// ---------------------------------------------------------------------------

/** Whether a finished operation changed the remote tree of the pane bound to
 *  `paneSessionId`. Uploads place bytes there; delete/chmod/chown mutate it.
 *  Downloads and local copies never touch it. */
export function refreshesRemotePane(
  kind: OperationKind,
  itemSessionId: string | undefined,
  paneSessionId: string,
): boolean {
  if (itemSessionId !== paneSessionId) return false;
  return kind === 'upload' || kind === 'delete' || kind === 'chmod' || kind === 'chown';
}

/** Whether a finished operation changed the local filesystem: a download
 *  writes into it, and so does a local copy (an Explorer drop). */
export function refreshesLocalPane(kind: OperationKind): boolean {
  return kind === 'download' || kind === 'localcopy';
}

/** The remote directories a finished operation made stale.
 *
 *  `refreshDir` alone is enough for every kind but a recursive chmod/chown:
 *  those rewrite the operated directory's own mode too, and that row is
 *  rendered by the parent's listing, so the parent is added. Deriving a parent
 *  from `refreshDir` is safe precisely because `refreshDir` is guaranteed to be
 *  a path — which is what `remotePath` never was. */
export function remoteRefreshDirs(kind: OperationKind, refreshDir: string): string[] {
  if (kind !== 'chmod' && kind !== 'chown') return [refreshDir];
  return [refreshDir, parentDir(refreshDir)];
}

/** Parent of a POSIX remote path, bottoming out at the root. */
function parentDir(remoteDir: string): string {
  return remoteDir.replace(/\/[^/]+$/, '') || '/';
}
