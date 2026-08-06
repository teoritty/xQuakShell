// The frontend half of the comment rules. Only the process-residue rule is
// mirrored here: the tautological-doc-comment rule in
// test/unit/architecture/comments.go keys off Go's doc-comment convention of
// starting with the identifier, which TypeScript has no equivalent of.

import { listFrontendFiles, readSource } from './budgetConfig';

/**
 * Matches the stage, phase or task number a piece of work was delivered under.
 * That is scheduling history: it tells a reader nothing about the code, and it
 * is wrong the moment the plan changes.
 *
 * "Step 3" is deliberately absent - numbered steps are how an algorithm gets
 * explained in order.
 */
export const PROCESS_RESIDUE = /\b(Stage|Phase|Tasks?)\s+\d/;

const COMMENT_RE = /\/\*[\s\S]*?\*\/|<!--[\s\S]*?-->|(?:^|[^:])(\/\/.*)$/gm;

/** Returns one message per offending comment; empty means the gate passes. */
export function checkComments(): string[] {
  const issues: string[] = [];
  for (const rel of listFrontendFiles()) {
    const src = readSource(rel);
    COMMENT_RE.lastIndex = 0;
    let match: RegExpExecArray | null;
    while ((match = COMMENT_RE.exec(src))) {
      const text = match[1] ?? match[0];
      const hit = PROCESS_RESIDUE.exec(text);
      if (!hit) continue;
      const line = src.slice(0, match.index).split('\n').length;
      issues.push(
        `${rel}:${line}: comment carries development-process history ("${hit[0]}"). ` +
          `Say what the code does or why; a stage number describes a schedule that no longer exists`
      );
    }
  }
  return issues;
}
