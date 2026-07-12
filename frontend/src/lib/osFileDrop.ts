// Handles files/folders dragged in from the OS file explorer (Wails OnFileDrop),
// as opposed to internal pane-to-pane drag-and-drop which uses HTML5 DataTransfer
// and is handled entirely within FileTree.svelte/LocalFileTree.svelte's own
// on:drop handlers. The two paths never collide: OS drops never carry the
// custom text/* MIME types internal drags use, and arrive exclusively through
// this Wails event instead of a DOM `drop` event.

export interface OsFileDropPayload {
  paths: string[];
  x: number;
  y: number;
}

type OsFileDropHandler = (payload: OsFileDropPayload) => void;

/**
 * Subscribes to the backend's OsFileDrop event. Returns an unsubscribe function.
 */
export function subscribeOsFileDrop(handler: OsFileDropHandler): () => void {
  const rt = (window as any).runtime;
  if (!rt) return () => {};
  const listener = (payload: OsFileDropPayload) => handler(payload);
  rt.EventsOn('OsFileDrop', listener);
  return () => rt.EventsOff('OsFileDrop');
}

function parentOf(path: string): string {
  const sep = path.includes('\\') ? '\\' : '/';
  const idx = path.lastIndexOf(sep);
  if (idx <= 0) return sep;
  return path.slice(0, idx);
}

/**
 * Resolves the drop target directory for an OS file drop landing at (x, y).
 * Returns null if the drop coordinates are outside `root` (not this pane).
 * Dropping on a folder row targets that folder; dropping on a file row or
 * empty area targets the file's parent / the pane's current directory.
 */
export function resolveOsDropTarget(root: HTMLElement, x: number, y: number, currentDir: string): string | null {
  const el = document.elementFromPoint(x, y);
  if (!el || !root.contains(el)) return null;
  const row = el.closest('.node-row[data-is-dir]');
  if (!row) return currentDir;
  const isDir = row.getAttribute('data-is-dir') === 'true';
  const path = row.getAttribute('data-path');
  if (!path) return currentDir;
  return isDir ? path : parentOf(path);
}

export function joinPath(dir: string, name: string): string {
  const sep = dir.includes('\\') && !dir.includes('/') ? '\\' : '/';
  if (dir === '/' || dir === '') return `${sep}${name}`;
  return dir.endsWith(sep) ? `${dir}${name}` : `${dir}${sep}${name}`;
}

export function baseName(path: string): string {
  return path.split(/[\\/]/).filter(Boolean).pop() || path;
}
