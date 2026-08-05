// Incremental search over a live log (ADR-015 §1).
//
// Two properties matter and neither is obvious: a repaint must not re-scan the whole buffer, and a
// hit must survive the buffer dropping older lines — which on a chatty producer happens constantly.
import { LogBuffer } from './buffer';
import { appendMatches, indexOfSeq, searchLines, stepMatch } from './search';

let failures = 0;
function assert(cond: boolean, msg: string) {
  if (!cond) {
    failures++;
    console.error('FAIL:', msg);
  }
}

const enc = (s: string) => new TextEncoder().encode(s);

function bufferWith(...texts: string[]): LogBuffer {
  const b = new LogBuffer();
  for (const t of texts) b.append(enc(t + '\n'), 'stdout');
  return b;
}

// --- matches are addressed by seq -------------------------------------------

{
  const b = bufferWith('alpha', 'beta', 'alpha again');
  const hits = searchLines(b.snapshot(), 'alpha', false);
  const seqs = b.snapshot().filter((l) => l.text.includes('alpha')).map((l) => l.seq);
  assert(hits.join(',') === seqs.join(','), `hits are line seqs, got ${hits.join(',')}`);
}

{
  // The regression: with positional hits, evicting the oldest line silently shifted every match
  // one line up, so "next match" jumped to the wrong row.
  const b = new LogBuffer(1 << 20, 3);
  for (const t of ['match one', 'filler', 'filler']) b.append(enc(t + '\n'), 'stdout');
  const hits = searchLines(b.snapshot(), 'match', false);
  const seqOfMatch = hits[0];

  b.append(enc('pushes the first line out\n'), 'stdout');
  const stillThere = b.snapshot().some((l) => l.seq === seqOfMatch);
  assert(!stillThere, 'the matching line was indeed evicted (test setup)');

  const after = appendMatches(hits, b.snapshot(), -1, 'match', false);
  assert(after.length === 0, `a hit whose line was evicted is dropped, got ${after.join(',')}`);
}

// --- incremental extension ---------------------------------------------------

{
  const b = bufferWith('hit one', 'miss');
  let hits = searchLines(b.snapshot(), 'hit', false);
  assert(hits.length === 1, 'first pass finds the existing match');

  const lastSeq = b.snapshot()[b.snapshot().length - 1].seq;
  b.append(enc('hit two\n'), 'stdout');

  hits = appendMatches(hits, b.snapshot(), lastSeq, 'hit', false);
  assert(hits.length === 2, `an appended match is added, got ${hits.length}`);
  assert(hits[0] < hits[1], 'matches stay in buffer order');
}

{
  // Only lines newer than afterSeq are examined — a line already counted is not counted twice.
  const b = bufferWith('hit', 'hit');
  const hits = searchLines(b.snapshot(), 'hit', false);
  const lastSeq = b.snapshot()[b.snapshot().length - 1].seq;
  const again = appendMatches(hits, b.snapshot(), lastSeq, 'hit', false);
  assert(again.length === 2, `re-running over the same lines must not duplicate, got ${again.length}`);
}

// --- case sensitivity and the empty query ------------------------------------

{
  const b = bufferWith('Alpha', 'ALPHA', 'beta');
  assert(searchLines(b.snapshot(), '', false).length === 0, 'an empty query matches nothing');
  assert(searchLines(b.snapshot(), 'alpha', false).length === 2, 'case-insensitive matches both');
  assert(searchLines(b.snapshot(), 'Alpha', true).length === 1, 'case-sensitive matches one');
  assert(appendMatches([1, 2], b.snapshot(), -1, '', false).length === 0, 'an empty query clears hits');
}

// --- addressing a match ------------------------------------------------------

{
  const b = bufferWith('a', 'b', 'c');
  const lines = b.snapshot();
  assert(indexOfSeq(lines, lines[1].seq) === 1, 'a seq resolves to its row');
  assert(indexOfSeq(lines, 999) === -1, 'an unknown seq resolves to nothing');
  assert(indexOfSeq([], 0) === -1, 'an empty buffer resolves nothing');
}

{
  const hits = [3, 8, 15];
  assert(stepMatch(hits, 0, 1) === 1, 'next match');
  assert(stepMatch(hits, 2, 1) === 0, 'next wraps at the end');
  assert(stepMatch(hits, 0, -1) === 2, 'previous wraps at the start');
  assert(stepMatch([], 0, 1) === -1, 'no matches means no position');
}

if (failures > 0) {
  console.error(`search.test: ${failures} failure(s)`);
  process.exit(1);
}
console.log('logSurface search tests passed');
