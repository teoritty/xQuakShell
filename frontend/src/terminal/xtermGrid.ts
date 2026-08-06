// Sizing an xterm grid to the box it was given.
//
// Extracted from Terminal.svelte, which is a renderer and had a layout problem living inside it.
// None of this is about which producer the terminal is wired to (ADR-015 gave it two), and all of
// it is about one WebView2 quirk documented on measureGrid.

import type { Terminal } from '@xterm/xterm';

/** Scrollbar gutter xterm reserves when scrollback is enabled (matches FitAddon). */
const SCROLLBAR_GUTTER = 14;

export interface GridSize {
  cols: number;
  rows: number;
}

/**
 * Measures cols/rows from the container's painted pixel box.
 *
 * FitAddon.proposeDimensions() reads getComputedStyle(parent).height, which in our flex layout
 * (WebView2) often reports ~40% of the real height on first paint — the grid stays ~80x24 and a
 * black bar sits below it. getBoundingClientRect() reflects the actual allocated flex area.
 *
 * Returns null whenever the answer would be a guess: no terminal, no painted box, or xterm has not
 * measured a cell yet.
 */
export function measureGrid(term: Terminal | null, containerEl: HTMLElement | null): GridSize | null {
  if (!term || !containerEl || !term.element) return null;
  const rect = containerEl.getBoundingClientRect();
  if (rect.width <= 0 || rect.height <= 0) return null;

  // reason: xterm exposes measured cell metrics only through its private core.
  const cell = (term as any)._core?._renderService?.dimensions?.css?.cell;
  if (!cell?.width || !cell?.height) return null;

  const xtermStyle = window.getComputedStyle(term.element);
  const padX =
    (parseFloat(xtermStyle.paddingLeft) || 0) + (parseFloat(xtermStyle.paddingRight) || 0);
  const padY =
    (parseFloat(xtermStyle.paddingTop) || 0) + (parseFloat(xtermStyle.paddingBottom) || 0);
  const gutter = term.options.scrollback === 0 ? 0 : SCROLLBAR_GUTTER;

  const cols = Math.max(2, Math.floor((rect.width - padX - gutter) / cell.width));
  const rows = Math.max(1, Math.floor((rect.height - padY) / cell.height));
  if (!isFinite(cols) || !isFinite(rows)) return null;
  return { cols, rows };
}

/**
 * Recomputes the grid to match its container.
 *
 * term.resize fires onResize, which is what pushes cols/rows to whatever is on the other end — a
 * PTY or a plugin. Doing nothing when the size is unchanged is therefore not just an optimisation:
 * it is what stops a resize event being sent for every repaint.
 */
export function refitGrid(
  term: Terminal | null,
  containerEl: HTMLElement | null,
  force = false
): void {
  if (!term || !containerEl) return;
  if (containerEl.offsetWidth <= 0 || containerEl.offsetHeight <= 0) return;
  try {
    const dims = measureGrid(term, containerEl);
    if (!dims) return;
    if (!force && dims.cols === term.cols && dims.rows === term.rows) return;
    term.resize(dims.cols, dims.rows);
  } catch {
    // A resize during teardown throws inside xterm; there is nothing to recover and nothing to say.
  }
}

/**
 * Keeps refitting until the grid catches up with the painted container.
 *
 * The first frames after mount report a container that is still growing, so a single fit lands on
 * a size the user never sees. This waits for two consecutive frames that agree on a sensible row
 * count, and gives up after ~90 frames rather than spinning if the layout never settles.
 */
export async function ensureInitialFit(
  term: Terminal | null,
  containerEl: HTMLElement | null
): Promise<void> {
  let stable = 0;
  let lastRows = 0;
  for (let i = 0; i < 90; i++) {
    await new Promise<void>((r) => requestAnimationFrame(() => r()));
    if (!term || !containerEl) return;
    if (containerEl.getBoundingClientRect().height <= 0) continue;
    const dims = measureGrid(term, containerEl);
    if (!dims) continue;
    if (dims.cols !== term.cols || dims.rows !== term.rows) {
      term.resize(dims.cols, dims.rows);
      stable = 0;
      lastRows = dims.rows;
      continue;
    }
    if (dims.rows === lastRows && dims.rows > 0) stable++;
    else lastRows = dims.rows;
    if (stable >= 2 && dims.rows >= 10) break;
  }
}
