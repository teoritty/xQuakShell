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

/**
 * The one pattern that finds a Svelte `<script>` block, for every measurement
 * that needs one.
 *
 * `\s*` before the closing `>` is load-bearing. HTML5 allows whitespace between
 * the tag name and the `>` of an end tag, so `</script >` and `</script\n>` are
 * real end tags that a browser and the Svelte compiler both honour. An end tag
 * this pattern misses does not fail loudly: the block simply runs past the end
 * of the file, matches nothing, and the component reports an empty script. That
 * is a hole straight through the script and function budgets - one stray space
 * and a component of any size stops being measured at all.
 *
 * A factory, not a shared constant: a `/g` regex carries a mutable `lastIndex`
 * between calls, so a single shared object makes every caller responsible for
 * resetting it and the one that forgets silently starts scanning mid-file.
 * Handing out a fresh object costs nothing and removes the failure mode instead
 * of documenting it.
 */
export function scriptBlockRe(): RegExp {
  return /<script\b[^>]*>([\s\S]*?)<\/script\s*>/gi;
}

/** Concatenates the contents of every <script> block in a Svelte component. */
export function svelteScript(src: string): string {
  const blocks: string[] = [];
  const re = scriptBlockRe();
  let match: RegExpExecArray | null;
  while ((match = re.exec(src))) {
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
