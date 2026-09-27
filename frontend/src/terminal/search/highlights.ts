// Marks search hits inside a terminal, the way an editor tints every match of its find box.
import type { IDisposable, Terminal } from '@xterm/xterm';
import { cellSpan } from './bufferText';
import type { SearchHit } from './searchRun';

// xterm accepts only #RRGGBB here. A muted amber for every match and a brighter one for the
// current, close to what VS Code uses, so the terminal and the result list read as one tool.
const MATCH_BG = '#623315';
const ACTIVE_BG = '#a36a0a';

/**
 * Each decoration is a DOM element xterm positions on every scroll, so a terminal with thousands
 * of matches would stutter. The result list still has every hit; only the tint is capped.
 */
const MAX_DECORATIONS = 500;

// The current match is drawn above the selection, because revealing it also selects it and a
// selection drawn over the tint would hide which match is current.
function decorateRow(term: Terminal, row: number, col: number, width: number, color: string): IDisposable | null {
  const buffer = term.buffer.active;
  const marker = term.registerMarker(row - (buffer.baseY + buffer.cursorY));
  if (!marker) return null;
  const layer = color === ACTIVE_BG ? 'top' : 'bottom';
  const decoration = term.registerDecoration({ marker, x: col, width, backgroundColor: color, layer });
  return {
    dispose: () => {
      decoration?.dispose();
      marker.dispose();
    },
  };
}

// A match that wraps onto the next row is two rectangles: a decoration is a single row high.
function decorateHit(term: Terminal, hit: SearchHit, color: string): IDisposable[] {
  const span = cellSpan(hit.line, hit.start, hit.end, term.cols);
  const out: IDisposable[] = [];
  let { row, col } = span;
  let remaining = span.length;
  while (remaining > 0) {
    const width = Math.min(remaining, term.cols - col);
    const d = decorateRow(term, row, col, width, color);
    if (d) out.push(d);
    remaining -= width;
    row += 1;
    col = 0;
  }
  return out;
}

/** Tints the hits in one terminal, the active one brighter. Returns the undo. */
export function highlightHits(term: Terminal, hits: SearchHit[], active: SearchHit | null): () => void {
  const disposables: IDisposable[] = [];
  for (const hit of hits.slice(0, MAX_DECORATIONS)) {
    if (hit === active) continue;
    disposables.push(...decorateHit(term, hit, MATCH_BG));
  }
  // Registered last so it wins where it overlaps a plain match.
  if (active) disposables.push(...decorateHit(term, active, ACTIVE_BG));
  // The DOM renderer paints a decoration's colour into the row itself and does not repaint the row
  // when the decoration goes, so without the refresh the previous current match stays bright until
  // something else happens to redraw that line.
  return () => {
    disposables.forEach((d) => d.dispose());
    term.refresh(0, term.rows - 1);
  };
}

/** Scrolls a hit into the middle of the terminal and selects it. */
export function revealHit(term: Terminal, hit: SearchHit): void {
  const span = cellSpan(hit.line, hit.start, hit.end, term.cols);
  term.scrollToLine(Math.max(0, span.row - Math.floor(term.rows / 2)));
  term.select(span.col, span.row, span.length);
}
