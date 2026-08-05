// Rendering a log buffer for export.
//
// Kept apart from the viewer because it answers a different question — what a saved file should
// contain, not what the screen should show — and because it is the part worth testing without a
// DOM around it.
import type { LogLine } from './buffer';

/**
 * Renders lines as text.
 *
 * `withStreamTags` prefixes stderr lines, because the colour that distinguishes them on screen
 * does not survive into a text file, and a saved log where errors are indistinguishable from
 * ordinary output is missing the thing it was most likely saved for.
 */
export function exportLines(lines: readonly LogLine[], withStreamTags: boolean): string {
  const out: string[] = [];
  for (const line of lines) {
    out.push(withStreamTags && line.stream === 'stderr' ? `[stderr] ${line.text}` : line.text);
  }
  return out.join('\n') + (out.length > 0 ? '\n' : '');
}

/**
 * Filename suggested for a saved log.
 *
 * The stem allows no dot at all, not merely no separator. A plugin chose this title, and keeping
 * dots would let one produce a stem containing `..` — which is a path traversal the moment the
 * suggestion reaches anything that joins it to a directory. There is nothing a dot in a log's
 * filename buys that is worth having to reason about that.
 */
export function exportFileName(title: string): string {
  const safe = title.replace(/[^A-Za-z0-9_-]+/g, '-').replace(/^-+|-+$/g, '') || 'log';
  const stamp = new Date().toISOString().replace(/[:.]/g, '-');
  return `${safe}-${stamp}.log`;
}
