// Search over a log surface's buffer.
//
// Plain substring matching, deliberately: a regex box over someone else's log output is a way to
// hang the UI on a pathological pattern, and the question a user actually has here is "where does
// this string appear".
import type { LogLine } from './buffer';

/** Indices into the snapshot, in order. Empty query matches nothing rather than everything. */
export function searchLines(
  lines: readonly LogLine[],
  query: string,
  caseSensitive: boolean
): number[] {
  if (query === '') return [];
  const needle = caseSensitive ? query : query.toLowerCase();
  const hits: number[] = [];
  for (let i = 0; i < lines.length; i++) {
    const haystack = caseSensitive ? lines[i].text : lines[i].text.toLowerCase();
    if (haystack.includes(needle)) hits.push(i);
  }
  return hits;
}

/** Steps through match indices, wrapping at both ends. Returns the new position in `hits`. */
export function stepMatch(hits: number[], current: number, delta: number): number {
  if (hits.length === 0) return -1;
  const next = (current + delta) % hits.length;
  return next < 0 ? next + hits.length : next;
}
