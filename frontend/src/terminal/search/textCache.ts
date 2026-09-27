// The searchable text of each terminal, extracted once and kept until the terminal changes.
//
// Reading a buffer cell by cell costs a few milliseconds per thousand rows, and the search re-runs
// on every keystroke in the query box. Extracting once per change of the terminal rather than once
// per keystroke is what keeps typing in the search box responsive with several busy terminals and
// full scrollback.
import type { Terminal } from '@xterm/xterm';
import { extractLines, type LogicalLine } from './bufferText';

interface CacheEntry {
  term: Terminal;
  lines: LogicalLine[] | null;
  dispose: () => void;
}

const cache = new Map<string, CacheEntry>();
const listeners = new Set<(terminalId: string) => void>();

function invalidate(terminalId: string): void {
  const entry = cache.get(terminalId);
  if (!entry || entry.lines === null) return;
  entry.lines = null;
  for (const listener of listeners) listener(terminalId);
}

// New output, a switch to or from the alternate screen (vim, htop, less) and a resize - which
// reflows wrapped rows - all change what a search would find.
function watch(terminalId: string, term: Terminal): CacheEntry {
  const subscriptions = [
    term.onWriteParsed(() => invalidate(terminalId)),
    term.buffer.onBufferChange(() => invalidate(terminalId)),
    term.onResize(() => invalidate(terminalId)),
  ];
  return { term, lines: null, dispose: () => subscriptions.forEach((s) => s.dispose()) };
}

/** The logical lines of a terminal's active buffer, extracted on first use after a change. */
export function linesOf(terminalId: string, term: Terminal): LogicalLine[] {
  let entry = cache.get(terminalId);
  if (!entry || entry.term !== term) {
    entry?.dispose();
    entry = watch(terminalId, term);
    cache.set(terminalId, entry);
  }
  if (entry.lines === null) entry.lines = extractLines(term.buffer.active);
  return entry.lines;
}

/** Drops every cached terminal that is no longer mounted. */
export function forgetClosedTerminals(live: ReadonlySet<string>): void {
  for (const [id, entry] of cache) {
    if (live.has(id)) continue;
    entry.dispose();
    cache.delete(id);
  }
}

/** Calls back when a cached terminal's text goes stale. Returns the unsubscribe. */
export function onTerminalTextChanged(listener: (terminalId: string) => void): () => void {
  listeners.add(listener);
  return () => listeners.delete(listener);
}
