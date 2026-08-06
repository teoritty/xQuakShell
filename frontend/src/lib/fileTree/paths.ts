// Path arithmetic for the file panes.
//
// The two panes do NOT share these rules and must not be made to: a remote
// listing is always POSIX because it comes over SFTP, while a local one is
// whatever the host uses, drive letters included. What they did share was
// having the rules inlined in an event handler where nothing could reach them.
// Both sets live here, named for the world they belong to, so neither can be
// applied to the wrong pane by accident.

// --- remote (always POSIX) ---

/** Parent of a POSIX path; the root is its own parent. */
export function remoteParent(path: string): string {
  return path.replace(/\/[^/]+$/, '') || '/';
}

/** Last segment of a POSIX path, or `fallback` when there is none. */
export function remoteBasename(path: string, fallback = 'item'): string {
  return path.split('/').filter(Boolean).pop() || fallback;
}

/** Join a POSIX directory and a name without doubling the root slash. */
export function remoteJoin(dir: string, name: string): string {
  return dir === '/' ? `/${name}` : `${dir}/${name}`;
}

/** What the user typed in the remote path bar, as an absolute POSIX path. */
export function normalizeRemotePathInput(input: string): string {
  const normalized = input.replace(/\\/g, '/').replace(/\/+/g, '/').replace(/\/$/, '') || '/';
  return normalized.startsWith('/') ? normalized : `/${normalized}`;
}

// --- local (POSIX or Windows, decided per path) ---

/** The separator a path is written with. A backslash anywhere means Windows. */
export function localSeparator(path: string): string {
  return path.includes('\\') ? '\\' : '/';
}

/** True at "/" or at a bare drive root such as "C:" or "C:\". */
export function isAtFilesystemRoot(path: string): boolean {
  const trimmed = path.replace(/[\\/]+$/, '');
  if (!trimmed || trimmed === '/') return true;
  return /^[a-zA-Z]:\\?$/i.test(trimmed);
}

/** Parent of a local path; a filesystem root is its own parent. */
export function parentDirectory(path: string): string {
  if (isAtFilesystemRoot(path)) return path;
  const trimmed = path.replace(/[\\/]+$/, '');
  const idx = Math.max(trimmed.lastIndexOf('\\'), trimmed.lastIndexOf('/'));
  if (/^[a-zA-Z]:/.test(trimmed)) {
    // Stop at the drive root rather than producing a bare "C:", which on
    // Windows names a per-drive working directory and not the same place as "C:\".
    if (idx <= 2) return `${trimmed.slice(0, 2)}\\`;
    return trimmed.slice(0, idx);
  }
  if (idx <= 0) return '/';
  return trimmed.slice(0, idx);
}

/** Last segment of a local path, either separator. */
export function localBasename(path: string, fallback = 'item'): string {
  return path.split(/[\\/]/).filter(Boolean).pop() || fallback;
}

/** Join a local directory and a name using the directory's own separator. */
export function localJoin(dir: string, name: string): string {
  const sep = localSeparator(dir);
  return dir.endsWith(sep) ? dir + name : dir + sep + name;
}

/** What the user typed in the local path bar, in the convention it was typed in. */
export function normalizeLocalPathInput(input: string, homeDir = ''): string {
  const looksWindows = input.includes('\\') || /^[a-zA-Z]:/.test(input);
  if (looksWindows) {
    let normalized = input.replace(/\//g, '\\').replace(/\\{2,}/g, '\\');
    if (/^[a-zA-Z]:$/.test(normalized)) return `${normalized}\\`;
    if (/^[a-zA-Z]:\\$/.test(normalized)) return normalized;
    normalized = normalized.replace(/\\+$/, '');
    return normalized || homeDir || '';
  }
  const normalized = input.replace(/\/{2,}/g, '/').replace(/\/+$/, '');
  return normalized || '/';
}
