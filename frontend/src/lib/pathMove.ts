// Pure helpers for validating a move/rename of a filesystem entry into a target
// directory. Shared by the local and remote file panes so both enforce the same
// rules. Works for POSIX ('/') and Windows ('\') paths — separators are
// normalised before comparison.

function normalize(path: string): string {
  // Collapse any run of separators to '/', drop a trailing separator (but keep a
  // bare root as '' so "/" and "C:\" degrade to a comparable empty prefix).
  return path.replace(/[\\/]+/g, '/').replace(/\/+$/, '');
}

function baseName(normPath: string): string {
  const parts = normPath.split('/').filter(Boolean);
  return parts.length > 0 ? parts[parts.length - 1] : '';
}

/**
 * True when moving `src` into directory `targetDir` is a no-op or illegal and
 * must be skipped silently (never surfaced as an error). Covers:
 *  - dropping onto the item's current location (dest would equal src),
 *  - dropping a folder onto itself,
 *  - dropping a folder into its own descendant (would create a cycle).
 * All three are normal user gestures in a FileZilla-style UI and should do nothing.
 */
export function isInvalidMove(src: string, targetDir: string): boolean {
  const s = normalize(src);
  const t = normalize(targetDir);
  if (s === '') return true; // guard against moving a root
  const dest = t === '' ? `/${baseName(s)}` : `${t}/${baseName(s)}`;
  return dest === s || t === s || t.startsWith(`${s}/`);
}
