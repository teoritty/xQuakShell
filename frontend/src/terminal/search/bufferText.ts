// Turns a terminal buffer into searchable text and maps a match back to the cells it came from.
//
// Two things make this more than joining translateToString() results. A long command that the
// terminal wrapped is one line to the person reading it, so rows flagged isWrapped are joined into
// the row before them; a search for a word split across the wrap must still find it. And a string
// offset is not a column: a wide character (CJK, most emoji) is one string character spread over
// two cells, so selecting "offset 10" on a line after a wide character would highlight the wrong
// cells. Rows are mapped cell by cell only when they need it - on the common ASCII row the string
// is exactly one character per cell and the offset IS the column.

/** The slice of xterm's IBufferCell this module reads. */
export interface CellLike {
  getChars(): string;
  getWidth(): number;
}

/** The slice of xterm's IBufferLine this module reads. */
export interface LineLike {
  readonly isWrapped: boolean;
  readonly length: number;
  translateToString(trimRight?: boolean): string;
  getCell(x: number): CellLike | undefined;
}

/** The slice of xterm's IBuffer this module reads. */
export interface BufferLike {
  readonly length: number;
  getLine(y: number): LineLike | undefined;
}

/** One buffer row's share of a logical line. */
interface RowSegment {
  row: number;
  /** Offset of this row's first character within the logical line's text. */
  start: number;
  text: string;
  /** Column and width of each character, or null when the row is one character per cell. */
  cols: number[] | null;
  widths: number[] | null;
  /** Cells the row spans; the width of its last character is measured against this. */
  width: number;
}

/** A line as the reader sees it: one or more buffer rows joined across soft wraps. */
export interface LogicalLine {
  text: string;
  /** The buffer row the line starts on, which is what a result is labelled with. */
  row: number;
  segments: RowSegment[];
}

/** A position in the buffer, in the coordinates xterm's select() and scrollToLine() take. */
export interface CellPosition {
  row: number;
  col: number;
}

function rowSegment(line: LineLike, row: number, start: number): RowSegment {
  const plain = line.translateToString(false);
  if (plain.length === line.length) {
    return { row, start, text: plain, cols: null, widths: null, width: line.length };
  }
  let text = '';
  const cols: number[] = [];
  const widths: number[] = [];
  for (let x = 0; x < line.length; x++) {
    const cell = line.getCell(x);
    // A wide character's second cell has width 0 and no characters of its own.
    if (!cell || cell.getWidth() === 0) continue;
    const chars = cell.getChars() || ' ';
    for (let i = 0; i < chars.length; i++) {
      cols.push(x);
      widths.push(cell.getWidth());
    }
    text += chars;
  }
  return { row, start, text, cols, widths, width: line.length };
}

// Trailing blanks are dropped from the last row only: every earlier row of a wrapped line is full
// by definition, and trimming one would shift every offset after it.
function finishLine(segments: RowSegment[]): LogicalLine {
  const last = segments[segments.length - 1];
  const trimmed = last.text.replace(/\s+$/, '');
  segments[segments.length - 1] = {
    ...last,
    text: trimmed,
    cols: last.cols?.slice(0, trimmed.length) ?? null,
    widths: last.widths?.slice(0, trimmed.length) ?? null,
  };
  return { text: segments.map((s) => s.text).join(''), row: segments[0].row, segments };
}

/** Reads every line of the buffer, scrollback included, as logical lines. */
export function extractLines(buffer: BufferLike): LogicalLine[] {
  const lines: LogicalLine[] = [];
  let current: RowSegment[] = [];
  let offset = 0;
  for (let y = 0; y < buffer.length; y++) {
    const line = buffer.getLine(y);
    if (!line) continue;
    if (!line.isWrapped && current.length > 0) {
      lines.push(finishLine(current));
      current = [];
      offset = 0;
    }
    const segment = rowSegment(line, y, offset);
    current.push(segment);
    offset += segment.text.length;
  }
  if (current.length > 0) lines.push(finishLine(current));
  return lines;
}

function segmentAt(line: LogicalLine, offset: number): RowSegment {
  let found = line.segments[0];
  for (const segment of line.segments) {
    if (segment.start > offset) break;
    found = segment;
  }
  return found;
}

/** The cell a character of a logical line sits in. */
export function cellAt(line: LogicalLine, offset: number): CellPosition {
  const segment = segmentAt(line, offset);
  const local = offset - segment.start;
  const col = segment.cols ? (segment.cols[local] ?? segment.width) : local;
  return { row: segment.row, col };
}

/**
 * The selection a match covers, as the start cell and the number of cells to select - the shape
 * Terminal.select() takes, which continues across rows.
 */
export function cellSpan(line: LogicalLine, start: number, end: number, cols: number): CellPosition & { length: number } {
  const lastOffset = Math.max(start, end - 1);
  const from = cellAt(line, start);
  const last = cellAt(line, lastOffset);
  const segment = segmentAt(line, lastOffset);
  const lastWidth = segment.widths?.[lastOffset - segment.start] ?? 1;
  const length = (last.row - from.row) * cols + (last.col - from.col) + lastWidth;
  return { ...from, length };
}
