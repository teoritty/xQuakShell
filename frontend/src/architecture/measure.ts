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
 * `(?:\s[^>]*)?` on the end tag is load-bearing, and it is wider than it first
 * looks it needs to be. An HTML5 end tag is `</`, the name, then anything up to
 * the `>`: whitespace and even attributes are a parse error the parser reports
 * and then ignores, so `</script >`, `</script\n>` and `</script\t\n foo="bar">`
 * all really do close the block in a browser and in the Svelte compiler.
 *
 * An end tag this pattern misses does not fail loudly. `[\s\S]*?` finds no
 * close, the match fails outright, and the component reports an empty script -
 * a hole straight through the script and function budgets, where one stray
 * space stops a component of any size being measured at all. Being permissive
 * here is the safe direction: closing a block early costs a mismeasurement,
 * never closing it costs the whole gate.
 *
 * The leading `\s` inside the group, rather than a bare `[^>]*`, is what keeps
 * `</scriptfoo>` from closing anything - that is a different tag, not this one
 * with junk after it.
 *
 * A factory, not a shared constant: a `/g` regex carries a mutable `lastIndex`
 * between calls, so a single shared object makes every caller responsible for
 * resetting it and the one that forgets silently starts scanning mid-file.
 * Handing out a fresh object costs nothing and removes the failure mode instead
 * of documenting it.
 */
export function scriptBlockRe(): RegExp {
  return /<script\b[^>]*>([\s\S]*?)<\/script(?:\s[^>]*)?>/gi;
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
