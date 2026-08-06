// Search over a log surface's buffer.
//
// Plain substring matching, deliberately: a regex box over someone else's log output is a way to
// hang the UI on a pathological pattern, and the question a user actually has here is "where does
// this string appear".
//
// Matches are addressed by `seq`, not by position. A position is only valid until the buffer drops
// its oldest lines, which on a live log happens continuously — a hit list of indices would quietly
// start pointing one line further up with every eviction.
import type { LogLine } from './buffer';

/** The seq of every matching line, in order. An empty query matches nothing rather than everything. */
export function searchLines(
  lines: readonly LogLine[],
  query: string,
  caseSensitive: boolean
): number[] {
  return appendMatches([], lines, -1, query, caseSensitive);
}

/**
 * Extends an existing hit list with matches from lines newer than `afterSeq`.
 *
 * This is what keeps a live log from being re-scanned end to end on every chunk: only the lines
 * that arrived since the last pass are examined, which is the difference between O(n) per write
 * and O(new). Hits whose lines have since been evicted are dropped in the same pass, so the list
 * never grows past the buffer it describes.
 */
export function appendMatches(
  hits: readonly number[],
  lines: readonly LogLine[],
  afterSeq: number,
  query: string,
  caseSensitive: boolean
): number[] {
  if (query === '') return [];
  const oldestSeq = lines.length > 0 ? lines[0].seq : Number.MAX_SAFE_INTEGER;
  const next = hits.filter((seq) => seq >= oldestSeq);
  const needle = caseSensitive ? query : query.toLowerCase();
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (line.seq <= afterSeq) continue;
    const haystack = caseSensitive ? line.text : line.text.toLowerCase();
    if (haystack.includes(needle)) next.push(line.seq);
  }
  return next;
}

/** Steps through match positions, wrapping at both ends. Returns the new position in `hits`. */
export function stepMatch(hits: readonly number[], current: number, delta: number): number {
  if (hits.length === 0) return -1;
  const next = (current + delta) % hits.length;
  return next < 0 ? next + hits.length : next;
}

/**
 * The index of the line carrying `seq`, or -1.
 *
 * Binary search, because seq is monotonic within the buffer and a live log is long: a linear scan
 * per jump would put the cost back that addressing by seq removed.
 */
export function indexOfSeq(lines: readonly LogLine[], seq: number): number {
  let lo = 0;
  let hi = lines.length - 1;
  while (lo <= hi) {
    const mid = (lo + hi) >> 1;
    if (lines[mid].seq === seq) return mid;
    if (lines[mid].seq < seq) lo = mid + 1;
    else hi = mid - 1;
  }
  return -1;
}
