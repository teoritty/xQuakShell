import { renameLocalPath } from '../../api/localFs';
import { renamePath } from '../../api/remoteFs';
import { isInvalidMove } from '../pathMove';
import { localBasename, localJoin, parentDirectory, remoteBasename, remoteJoin, remoteParent } from './paths';

/** The three path rules and the one call that differ between the two panes. */
interface MoveFilesystem {
  join(dir: string, name: string): string;
  basename(path: string): string;
  parent(path: string): string;
  rename(from: string, to: string): Promise<void>;
}

export interface MoveResult {
  /** Directories the moves emptied, so the caller knows which listings went stale. */
  sourceParents: string[];
  /** The first failure's message, or '' when every move landed. */
  error: string;
}

/**
 * Moves dragged paths into a directory, on whichever filesystem the caller names.
 *
 * The two panes are near-clones by design — one over SFTP, one over the host filesystem — and this
 * loop was written out twice, identically, in both. Only three things actually differ: how a path
 * is joined, how its parent is taken, and which rename is called.
 *
 * A move that would land a directory inside itself is skipped rather than attempted: the rename
 * would either fail with an error the user cannot act on or, worse, succeed and detach the subtree.
 *
 * One failure does not stop the batch. A drag of forty files where the third is locked should move
 * the other thirty-nine, and the message from the first failure is what the pane shows.
 */
async function movePaths(
  paths: readonly string[],
  targetDir: string,
  fs: MoveFilesystem,
): Promise<MoveResult> {
  const sourceParents: string[] = [];
  let error = '';
  for (const path of paths) {
    if (isInvalidMove(path, targetDir)) continue;
    try {
      await fs.rename(path, fs.join(targetDir, fs.basename(path)));
      const from = fs.parent(path);
      if (!sourceParents.includes(from)) sourceParents.push(from);
    } catch (e) {
      error = error || (e as { message?: string })?.message || String(e);
    }
  }
  return { sourceParents, error };
}

export function moveLocalPaths(paths: readonly string[], targetDir: string): Promise<MoveResult> {
  return movePaths(paths, targetDir, {
    join: localJoin,
    basename: localBasename,
    parent: parentDirectory,
    rename: renameLocalPath,
  });
}

/** The remote half. A move is only ever within one session, which is why the id is not per path. */
export function moveRemotePaths(
  sessionId: string,
  paths: readonly string[],
  targetDir: string,
): Promise<MoveResult> {
  return movePaths(paths, targetDir, {
    join: remoteJoin,
    basename: remoteBasename,
    parent: remoteParent,
    rename: (from, to) => renamePath(sessionId, from, to),
  });
}
