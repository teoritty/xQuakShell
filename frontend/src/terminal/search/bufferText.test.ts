import { extractLines, cellAt, cellSpan, type BufferLike, type CellLike, type LineLike } from './bufferText';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

/** One buffer row, built from cells the way xterm lays them out: a wide char is a width-2 cell
 *  followed by an empty width-0 cell. */
function row(cells: { chars: string; width: number }[], cols: number, isWrapped = false): LineLike {
  const padded = [...cells];
  while (padded.length < cols) padded.push({ chars: '', width: 1 });
  return {
    isWrapped,
    length: cols,
    translateToString: () => padded.map((c) => (c.width === 0 ? '' : c.chars || ' ')).join(''),
    getCell: (x): CellLike | undefined => {
      const c = padded[x];
      return c ? { getChars: () => c.chars, getWidth: () => c.width } : undefined;
    },
  };
}

function ascii(text: string, cols: number, isWrapped = false): LineLike {
  return row([...text].map((chars) => ({ chars, width: 1 })), cols, isWrapped);
}

function buffer(lines: LineLike[]): BufferLike {
  return { length: lines.length, getLine: (y) => lines[y] };
}

// Plain rows: one logical line each, trailing blanks dropped, and an offset is its own column.
{
  const lines = extractLines(buffer([ascii('$ ls -la', 20), ascii('total 8', 20)]));
  assert(lines.length === 2, `two unwrapped rows are two lines, got ${lines.length}`);
  assert(lines[0].text === '$ ls -la', `trailing blanks trimmed, got ${JSON.stringify(lines[0].text)}`);
  assert(lines[1].row === 1, 'a line is labelled with the row it starts on');
  const at = cellAt(lines[0], 5);
  assert(at.row === 0 && at.col === 5, `ASCII offset 5 is column 5, got ${JSON.stringify(at)}`);
}

// A wrapped command is one line to the reader; a word split by the wrap must still be one string.
{
  const lines = extractLines(buffer([ascii('echo hel', 8), ascii('lo world', 8, true), ascii('next', 8)]));
  assert(lines.length === 2, `a wrapped row joins the row before it, got ${lines.length} lines`);
  assert(lines[0].text === 'echo hello world', `joined text, got ${JSON.stringify(lines[0].text)}`);
  const at = cellAt(lines[0], 'echo hel'.length);
  assert(at.row === 1 && at.col === 0, `first char after the wrap is row 1 col 0, got ${JSON.stringify(at)}`);
  const span = cellSpan(lines[0], 5, 10, 8); // "hello", across the wrap
  assert(span.row === 0 && span.col === 5 && span.length === 5,
    `a match across the wrap selects 5 cells from row 0 col 5, got ${JSON.stringify(span)}`);
  assert(lines[1].row === 2, 'the line after a wrapped one starts at its own row');
}

// A wide character takes two cells, so every column after it is one more than its string offset.
{
  const cells = [
    { chars: '日', width: 2 },
    { chars: '', width: 0 },
    { chars: 'a', width: 1 },
    { chars: 'b', width: 1 },
  ];
  const [line] = extractLines(buffer([row(cells, 10)]));
  assert(line.text === '日ab', `the empty second cell of a wide char adds nothing, got ${JSON.stringify(line.text)}`);
  const b = cellAt(line, 2);
  assert(b.col === 3, `'b' is string offset 2 but column 3 after a wide char, got ${b.col}`);
  const span = cellSpan(line, 0, 1, 10);
  assert(span.length === 2, `selecting a wide char selects both its cells, got ${span.length}`);
}

// An empty buffer and blank rows do not produce phantom text.
{
  assert(extractLines(buffer([])).length === 0, 'an empty buffer has no lines');
  const [blank] = extractLines(buffer([ascii('', 10)]));
  assert(blank.text === '', `a blank row is an empty line, got ${JSON.stringify(blank.text)}`);
}

console.log('bufferText.test passed');
