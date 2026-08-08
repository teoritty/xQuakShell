/**
 * Width bounds for the connection sidebar, kept out of the component so the arithmetic can be
 * tested without a DOM. The session view's file column resizes by flex ratio and needs none of
 * this; the sidebar is a fixed-width column, so its bounds have to be stated in pixels.
 */

/**
 * The width the sidebar had when it could not be resized at all. It is the starting width and the
 * floor in one number: the layout was designed around it, and narrower truncates connection names
 * to the point where the tree stops being usable. Dragging only ever widens.
 */
export const MIN_SIDEBAR_WIDTH = 280;

/** Past this the sidebar is just wasting space on names that ran out of characters long ago. */
const MAX_SIDEBAR_WIDTH = 640;

/**
 * Clamps a dragged width to what the window can actually give up. The ceiling is half the viewport
 * because the terminal is the reason the app is open; a sidebar that can be dragged over it turns
 * one careless drag into a window the user has to fight back. The floor wins over that ceiling on a
 * window narrower than twice the minimum, since the alternative is a sidebar below its own minimum.
 */
export function clampSidebarWidth(desired: number, viewportWidth: number): number {
  // Only NaN is special-cased. An infinite width is a drag pinned to an edge and clamps to the
  // bound on that side like any other overshoot; NaN is a measurement that never happened, and
  // there is no bound to pick from it.
  if (Number.isNaN(desired)) return MIN_SIDEBAR_WIDTH;
  const half = Number.isFinite(viewportWidth) ? Math.floor(viewportWidth / 2) : MAX_SIDEBAR_WIDTH;
  const ceiling = Math.min(MAX_SIDEBAR_WIDTH, half);
  // The floor is applied after the ceiling, which is what settles the case where the two cross on
  // a window narrower than twice the minimum: the sidebar keeps its minimum and the window is the
  // one that has to give. Guarding the ceiling against dropping below the floor as well would read
  // as belt and braces and be neither - the second clamp already decides it.
  return Math.max(MIN_SIDEBAR_WIDTH, Math.min(ceiling, Math.round(desired)));
}
