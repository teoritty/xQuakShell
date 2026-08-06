// Measuring code for the frontend size budgets.
//
// The counting rule matches countGoCodeLines in test/unit/architecture: a line
// counts when it carries something other than a comment. Comments are free so
// that a budget never pushes an author to delete the explanation instead of
// the complexity.

/**
 * Removes block and line comments so comment-only mentions of a banned pattern
 * stop matching.
 *
 * The block strip runs to a fixpoint: removing one match splices its
 * neighbours together, and `/` + `*` on either side of the removal re-forms an
 * opening delimiter a single pass would miss. Line comments are anchored to a
 * line end and cannot re-form, so one pass is enough there. The `[^:]` guard
 * keeps `https://` inside a string from being read as a comment.
 */
export function stripComments(src: string): string {
  let prev: string;
  let out = src;
  do {
    prev = out;
    out = out.replace(/\/\*[\s\S]*?\*\//g, '');
  } while (out !== prev);
  return out.replace(/(^|[^:])\/\/.*$/gm, '$1');
}

/**
 * Blanks out comments while keeping every newline, so a line number in the
 * result still points at the same line of the original. Counting needs that;
 * stripComments does not preserve it, because removing a block comment splices
 * its two neighbouring lines into one.
 *
 * Blanking also removes the fixpoint problem stripComments has to loop for: a
 * replaced comment leaves spaces behind, and spaces cannot re-form a
 * delimiter. HTML comments are handled too - they are the Svelte markup half
 * of the same rule.
 */
export function blankComments(src: string): string {
  const blank = (m: string): string => m.replace(/[^\n]/g, ' ');
  return src
    .replace(/<!--[\s\S]*?-->/g, blank)
    .replace(/\/\*[\s\S]*?\*\//g, blank)
    .replace(/(^|[^:])\/\/.*$/gm, '$1');
}

export function countCodeLines(src: string): number {
  return blankComments(src)
    .split('\n')
    .filter((line) => line.trim() !== '').length;
}

const SCRIPT_BLOCK_RE = /<script\b[^>]*>([\s\S]*?)<\/script>/gi;

/** Concatenates the contents of every <script> block in a Svelte component. */
export function svelteScript(src: string): string {
  const blocks: string[] = [];
  let match: RegExpExecArray | null;
  SCRIPT_BLOCK_RE.lastIndex = 0;
  while ((match = SCRIPT_BLOCK_RE.exec(src))) {
    blocks.push(match[1]);
  }
  return blocks.join('\n');
}

export interface FileMeasurement {
  codeLines: number;
  /** Present for Svelte components only. */
  scriptCodeLines?: number;
}

export function measureSource(relPath: string, src: string): FileMeasurement {
  if (!relPath.endsWith('.svelte')) {
    return { codeLines: countCodeLines(src) };
  }
  return {
    codeLines: countCodeLines(src),
    scriptCodeLines: countCodeLines(svelteScript(src)),
  };
}
