/**
 * Height bounds for the sidebar's bottom panel — the connection form, a discovery node's
 * properties, and any panel a plugin contributes there. Kept out of the component so the arithmetic
 * can be tested without a DOM, the same way sidebarWidth.ts is.
 *
 * What the drag moves is a ceiling, not a height. The panel is content-sized and scrolls inside
 * that ceiling, so a short form stays short and only a long one grows to fill what it is given.
 * Setting a height instead would leave an empty box below a two-field form.
 */

/** Below this the panel shows its header and almost nothing else, which is not worth dragging to. */
export const MIN_PANEL_HEIGHT = 140;

/**
 * What the tree keeps for itself no matter how far the panel is dragged. A tree squeezed to a
 * couple of rows cannot be navigated, and the panel is opened *from* the tree — trading the tree
 * away to see more of the panel breaks the loop the user is in.
 */
const MIN_TREE_HEIGHT = 180;

/** Past this the panel has stopped being a detail pane and has become the sidebar. */
const MAX_PANEL_FRACTION = 0.8;

/**
 * Clamps a dragged panel ceiling to what the window can give up.
 *
 * The floor is applied after the ceiling, which settles the case where the two cross on a short
 * window: the panel keeps its minimum and the tree is the one that gives. That is the same
 * resolution clampSidebarWidth documents for its own crossing bounds, and it is deliberate — a
 * panel below its own minimum is unusable, while a squeezed tree is merely uncomfortable.
 */
export function clampPanelHeight(desired: number, viewportHeight: number): number {
  // Only NaN is special-cased, matching clampSidebarWidth. An infinite height is a drag pinned to
  // an edge and clamps to the bound on that side like any other overshoot; NaN is a measurement
  // that never happened, and there is no bound to pick from it.
  if (Number.isNaN(desired)) return MIN_PANEL_HEIGHT;
  const ceiling = Number.isFinite(viewportHeight)
    ? Math.min(Math.floor(viewportHeight * MAX_PANEL_FRACTION), viewportHeight - MIN_TREE_HEIGHT)
    : MIN_PANEL_HEIGHT;
  return Math.max(MIN_PANEL_HEIGHT, Math.min(ceiling, Math.round(desired)));
}
