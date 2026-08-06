// Routing for files/folders dragged in from the OS file explorer.
//
// Native OS drops arrive through Wails' front-end bridge: window.runtime.OnFileDrop
// resolves the dropped items to absolute paths in the Go layer and invokes our
// callback with (x, y, paths) in CSS pixels. This module owns that bridge exactly
// once and routes each drop to whichever registered zone sits under the drop
// point, and it draws the drag-over highlight itself. There is deliberately NO
// round-trip back through the Go event bus and NO dependency on the
// `--wails-drop-target` CSS property: delivery is decided purely by hit-testing
// the registered zones, which removes every layer that could silently swallow a
// drop.
//
// Internal pane-to-pane drags use HTML5 DataTransfer and never reach this module:
// Wails only forwards drags whose dataTransfer contains the "Files" type (i.e.
// real OS files), so the two paths never collide.

export interface OsDropZone {
  /** Root element of the pane. A drop is routed here when it lands inside it. */
  el: HTMLElement;
  /** Called with the dropped absolute paths and CSS-pixel drop coordinates. */
  onDrop: (paths: string[], x: number, y: number) => void;
}

const ACTIVE_CLASS = 'os-drop-active';

const zones = new Set<OsDropZone>();
let bridgeReady = false;
let highlighted: HTMLElement | null = null;

// Last pointer position observed on a DOM drag/drop event, in CSS pixels. These
// DOM coordinates are the authoritative source of "where" the drop happened:
// the (x, y) Wails passes to its OnFileDrop callback can be off (e.g. under
// Windows display scaling), which would misresolve the folder under the cursor.
// The Wails callback is used only for the resolved file *paths*.
let lastPointerX: number | null = null;
let lastPointerY: number | null = null;

/** Finds the registered zone containing `el` (an element hit-tested from a point). */
function zoneFor(el: Element | null): OsDropZone | null {
  if (!el) return null;
  for (const zone of zones) {
    if (zone.el.contains(el)) return zone;
  }
  return null;
}

function clearHighlight() {
  if (highlighted) {
    highlighted.classList.remove(ACTIVE_CLASS);
    highlighted = null;
  }
}

function setHighlight(el: HTMLElement | null) {
  if (highlighted === el) return;
  clearHighlight();
  if (el) {
    el.classList.add(ACTIVE_CLASS);
    highlighted = el;
  }
}

/**
 * True when the drag carries real OS files (dragged in from the file explorer)
 * as opposed to an internal pane-to-pane drag. This is the single source of
 * truth panes use to hand external drops off to this router instead of their own
 * HTML5 handlers.
 */
export function isFileDrag(e: DragEvent): boolean {
  return !!e.dataTransfer && Array.from(e.dataTransfer.types).includes('Files');
}

/**
 * DataTransfer MIME types set by the remote/local file-tree panes on dragstart.
 * These identify an internal file move/upload/download drag.
 */
const FILE_PANE_DRAG_TYPES = [
  'text/remote-path',
  'text/selected-paths',
  'text/local-path',
  'text/local-selected-paths',
] as const;

/**
 * True when the drag originates from a file-tree pane (a remote/local file being
 * moved, uploaded or downloaded). Panes use this to claim only their own drags:
 * any other internal drag (e.g. a tile tab dragged between tiles) must be left to
 * bubble so the surrounding layout can handle it.
 */
export function isInternalFileDrag(e: DragEvent): boolean {
  if (!e.dataTransfer) return false;
  const types = Array.from(e.dataTransfer.types);
  return FILE_PANE_DRAG_TYPES.some((t) => types.includes(t));
}

/**
 * Resolves the element to highlight for a pointer at (x, y): the folder row under
 * the cursor when there is one (FileZilla-style per-folder target), otherwise the
 * pane itself (drop lands in the pane's current directory). Returns null outside
 * any zone.
 */
function highlightTargetAt(x: number, y: number): HTMLElement | null {
  const el = document.elementFromPoint(x, y);
  const zone = zoneFor(el);
  if (!zone) return null;
  const row = el?.closest('.node-row[data-is-dir="true"]') as HTMLElement | null;
  return row ?? zone.el;
}

function onDragOver(e: DragEvent) {
  if (!isFileDrag(e)) return;
  // Ensure the browser permits the subsequent drop regardless of window
  // listener ordering (a drop only fires if dragover was default-prevented).
  e.preventDefault();
  lastPointerX = e.clientX;
  lastPointerY = e.clientY;
  setHighlight(highlightTargetAt(e.clientX, e.clientY));
}

function onDragLeave(e: DragEvent) {
  if (!isFileDrag(e)) return;
  // Only clear when the cursor actually leaves the window; dragleave also fires
  // when moving between child elements, where relatedTarget stays non-null.
  if (!e.relatedTarget) clearHighlight();
}

function onDomDrop(e: DragEvent) {
  if (!isFileDrag(e)) return;
  // Capture the authoritative drop coordinates from the DOM event; the Wails
  // OnFileDrop callback (which carries the paths) fires slightly later.
  lastPointerX = e.clientX;
  lastPointerY = e.clientY;
  clearHighlight();
}

/**
 * Delivers a resolved OS drop to the zone under the drop point. `fallbackX/Y` are
 * Wails' own coordinates, used only if no DOM event was seen (should not happen).
 */
function routeDrop(fallbackX: number, fallbackY: number, paths: string[]) {
  clearHighlight();
  if (!paths || paths.length === 0) return;
  const x = lastPointerX ?? fallbackX;
  const y = lastPointerY ?? fallbackY;
  lastPointerX = null;
  lastPointerY = null;
  zoneFor(document.elementFromPoint(x, y))?.onDrop(paths, x, y);
}

/**
 * Wires up the Wails OnFileDrop bridge and our own highlight listeners, exactly
 * once. `useDropTarget` is false so Wails delivers every OS drop and we decide
 * routing ourselves via hit-testing — no reliance on inherited CSS custom
 * properties. Returns true once the bridge is live. The Wails runtime is normally
 * present before any component mounts; if it is not yet, this is retried on the
 * next zone registration.
 */
function ensureBridge(): boolean {
  if (bridgeReady) return true;
  const rt = (window as any).runtime;
  if (typeof rt?.OnFileDrop !== 'function') return false;
  bridgeReady = true;
  rt.OnFileDrop((x: number, y: number, paths: string[]) => routeDrop(x, y, paths), false);
  window.addEventListener('dragover', onDragOver);
  window.addEventListener('dragleave', onDragLeave);
  window.addEventListener('drop', onDomDrop);
  return true;
}

/**
 * Registers a pane as an OS file-drop target. Returns an unsubscribe function
 * that removes only this zone (never touching other panes' registrations).
 */
export function registerOsDropZone(zone: OsDropZone): () => void {
  zones.add(zone);
  if (!ensureBridge()) {
    // Runtime not ready yet — retry shortly so the pane still becomes a target.
    const timer = window.setInterval(() => {
      if (ensureBridge()) window.clearInterval(timer);
    }, 100);
    window.setTimeout(() => window.clearInterval(timer), 5000);
  }
  return () => {
    zones.delete(zone);
    if (highlighted === zone.el) clearHighlight();
  };
}

function parentOf(path: string): string {
  const sep = path.includes('\\') ? '\\' : '/';
  const idx = path.lastIndexOf(sep);
  if (idx <= 0) return sep;
  return path.slice(0, idx);
}

/**
 * Resolves the drop target directory for an OS file drop landing at (x, y),
 * where (x, y) are CSS pixels. Returns null if the point is outside `root`.
 * Dropping on a folder row targets that folder; dropping on a file row or empty
 * area targets the file's parent / the pane's current directory.
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

