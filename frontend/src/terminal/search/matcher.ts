// Compiles what the user typed into a matcher and runs it over one line.

export interface SearchOptions {
  caseSensitive: boolean;
  wholeWord: boolean;
  regex: boolean;
}

export type CompiledQuery = { ok: true; re: RegExp } | { ok: false; error: string };

export interface TextRange {
  start: number;
  end: number;
}

function escapeRegExp(text: string): string {
  return text.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

/**
 * Builds the regular expression for a query, or the reason it cannot be built.
 *
 * Always compiled with the `u` flag. Without it `\b` and `\w` know only ASCII, so a whole-word
 * search for a Russian word would match inside another word, and an emoji would be matched as two
 * halves. Whole word is spelled with Unicode lookarounds for the same reason - JavaScript's `\b` is
 * ASCII-only even under `u`.
 *
 * An empty query compiles to nothing rather than to a pattern that matches everywhere.
 */
export function compileQuery(query: string, options: SearchOptions): CompiledQuery | null {
  if (!query) return null;
  let source = options.regex ? query : escapeRegExp(query);
  if (options.wholeWord) source = `(?<![\\p{L}\\p{N}_])(?:${source})(?![\\p{L}\\p{N}_])`;
  const flags = options.caseSensitive ? 'gu' : 'giu';
  try {
    return { ok: true, re: new RegExp(source, flags) };
  } catch (e) {
    return { ok: false, error: e instanceof Error ? e.message : String(e) };
  }
}

/**
 * Every match of re in text, up to limit.
 *
 * A pattern that can match the empty string - `a*`, `^`, a lookahead - would otherwise report a
 * zero-width hit at every position and never advance. Such matches are skipped, and lastIndex is
 * stepped past them by a whole code point so a surrogate pair is never split.
 */
export function findInText(text: string, re: RegExp, limit: number): TextRange[] {
  const ranges: TextRange[] = [];
  re.lastIndex = 0;
  while (ranges.length < limit) {
    const m = re.exec(text);
    if (!m) break;
    if (m[0].length === 0) {
      const code = text.codePointAt(re.lastIndex);
      re.lastIndex += code !== undefined && code > 0xffff ? 2 : 1;
      if (re.lastIndex > text.length) break;
      continue;
    }
    ranges.push({ start: m.index, end: m.index + m[0].length });
  }
  return ranges;
}
