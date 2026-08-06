// Which slice of a log a viewport is actually showing.
//
// A log surface holds up to MAX_LOG_LINES entries, and rendering them all is a DOM node per line:
// the browser stops being usable long before the buffer reaches its limit. Only the visible rows
// are rendered, with a spacer above and a total height below, so the scrollbar still describes the
// whole log.
//
// Pure arithmetic, kept out of the component so it can be tested without a DOM.

export interface LogWindow {
  /** Index of the first rendered line. */
  first: number;
  /** How many lines to render. */
  count: number;
  /** Pixels of empty space standing in for the lines above `first`. */
  topPad: number;
  /** Height of the whole log, so the scrollbar matches the content. */
  totalHeight: number;
}

/**
 * Rows rendered beyond each edge of the viewport.
 *
 * Without an overscan, a fast scroll paints blank rows for a frame before the next window is
 * computed; a handful of extra rows costs nothing and removes the flicker.
 */
export const LOG_WINDOW_OVERSCAN = 12;

/**
 * Computes the window for a viewport.
 *
 * rowHeight must be the real rendered line height — the viewer fixes it in CSS (no wrapping)
 * precisely so this arithmetic is possible: with variable-height rows there is no way to know
 * which line sits at a given scroll offset without measuring every one of them.
 */
export function computeLogWindow(
  totalLines: number,
  scrollTop: number,
  viewportHeight: number,
  rowHeight: number
): LogWindow {
  if (rowHeight <= 0 || totalLines <= 0) {
    return { first: 0, count: 0, topPad: 0, totalHeight: 0 };
  }
  const totalHeight = totalLines * rowHeight;
  const visibleRows = Math.ceil(Math.max(viewportHeight, 0) / rowHeight) + 1;
  const firstVisible = Math.floor(Math.max(scrollTop, 0) / rowHeight);

  const first = Math.max(0, Math.min(firstVisible - LOG_WINDOW_OVERSCAN, Math.max(0, totalLines - 1)));
  const count = Math.min(visibleRows + LOG_WINDOW_OVERSCAN * 2, totalLines - first);

  return { first, count, topPad: first * rowHeight, totalHeight };
}

/** Scroll offset that puts a line in the middle of the viewport, clamped to the scrollable range. */
export function scrollTopForLine(
  index: number,
  totalLines: number,
  viewportHeight: number,
  rowHeight: number
): number {
  if (rowHeight <= 0 || index < 0) return 0;
  const centred = index * rowHeight - Math.max(viewportHeight - rowHeight, 0) / 2;
  const maxScroll = Math.max(totalLines * rowHeight - viewportHeight, 0);
  return Math.max(0, Math.min(centred, maxScroll));
}
