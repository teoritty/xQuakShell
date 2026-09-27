// Runs one compiled query over the text of several terminals and shapes the results for the panel.
import type { LogicalLine } from './bufferText';
import { findInText } from './matcher';

export interface SearchHit {
  terminalId: string;
  line: LogicalLine;
  start: number;
  end: number;
}

export interface TerminalHits {
  terminalId: string;
  hits: SearchHit[];
}

export interface SearchOutcome {
  groups: TerminalHits[];
  total: number;
  /** True when the cap cut the results short; the panel says so rather than implying it saw all. */
  truncated: boolean;
}

/**
 * Enough to find anything a person would scroll a result list for. Past it the list stops being
 * navigable and every re-run on new output costs more than the answer is worth.
 */
export const MAX_RESULTS = 2000;

/** Searches each terminal's lines in the order given, which is the order the panel lists them. */
export function runSearch(
  sources: { terminalId: string; lines: LogicalLine[] }[],
  re: RegExp,
  cap: number = MAX_RESULTS
): SearchOutcome {
  const groups: TerminalHits[] = [];
  let total = 0;
  let truncated = false;
  for (const { terminalId, lines } of sources) {
    const hits: SearchHit[] = [];
    for (const line of lines) {
      if (total >= cap) {
        truncated = true;
        break;
      }
      for (const range of findInText(line.text, re, cap - total)) {
        hits.push({ terminalId, line, start: range.start, end: range.end });
        total++;
      }
    }
    if (hits.length > 0) groups.push({ terminalId, hits });
  }
  return { groups, total, truncated };
}

export interface Preview {
  before: string;
  match: string;
  after: string;
}

/**
 * The part of a hit's line the result list shows: the match with some context either side.
 *
 * A terminal line can be thousands of characters of minified JSON or a base64 blob, and the list
 * row is one line wide. The context before the match is kept short on purpose: the row is cut at
 * its right edge, and a long lead-in pushes the match itself past that edge - which happens at once
 * in a sidebar the width of this one. Leading indentation is dropped for the same reason.
 */
export function previewOf(hit: SearchHit, leadIn = 12, tail = 120): Preview {
  const text = hit.line.text;
  const from = Math.max(0, hit.start - leadIn);
  const to = Math.min(text.length, hit.end + tail);
  // Cutting off nothing but indentation is not cutting anything the reader would miss.
  const cut = text.slice(0, from).trim() !== '';
  const before = (cut ? '…' : '') + text.slice(from, hit.start).replace(/^\s+/, '');
  const after = text.slice(hit.end, to) + (to < text.length ? '…' : '');
  return { before, match: text.slice(hit.start, hit.end), after };
}

/** The hits of every group in list order, which is the order Enter and Shift+Enter walk. */
export function flattenHits(groups: TerminalHits[]): SearchHit[] {
  return groups.flatMap((g) => g.hits);
}
