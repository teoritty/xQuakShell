// Singleton coordinator ensuring only one context menu is open at a time.
//
// Each context menu is owned by its own component and is normally dismissed by
// a window `click` handler. A right-click (`contextmenu`) in a *different*
// component never emits a `click`, so without coordination the previously
// opened menu stays visible and menus stack up. Every menu registers its close
// callback here on open; opening a new one first closes the previous one.

type CloseFn = () => void;

let activeClose: CloseFn | null = null;

/** Register a newly opened menu, closing any other menu that was open. */
export function openContextMenu(close: CloseFn): void {
  if (activeClose && activeClose !== close) {
    const prev = activeClose;
    activeClose = null;
    prev();
  }
  activeClose = close;
}

/** Deregister a menu when it closes so we don't hold a stale callback. */
export function releaseContextMenu(close: CloseFn): void {
  if (activeClose === close) activeClose = null;
}
