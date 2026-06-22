export interface MenuAnchorRect {
  left: number;
  top: number;
  right: number;
  bottom: number;
}

export function clampMenuPosition(
  anchor: MenuAnchorRect,
  menuWidth: number,
  menuHeight: number,
  padding = 8
): { left: number; top: number } {
  const maxLeft = window.innerWidth - menuWidth - padding;
  const maxTop = window.innerHeight - menuHeight - padding;

  let left = anchor.left;
  let top = anchor.bottom + 2;

  if (left + menuWidth > window.innerWidth - padding) {
    left = anchor.right - menuWidth;
  }
  left = Math.max(padding, Math.min(left, maxLeft));

  if (top + menuHeight > window.innerHeight - padding) {
    top = anchor.top - menuHeight - 2;
  }
  top = Math.max(padding, Math.min(top, maxTop));

  return { left, top };
}
