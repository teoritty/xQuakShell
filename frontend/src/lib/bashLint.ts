/**
 * The three unterminated-construct checks the script editor runs as you type.
 *
 * It is not a shell parser and does not try to be: it answers one question — would this text end
 * mid-quote or mid-heredoc — because a command pasted with a quote missing is the one that hangs a
 * terminal waiting for input rather than failing.
 */

/** The keys of the problems found, in the order they are reported. */
export type BashLintProblem =
  | 'scripts.lint.singleQuote'
  | 'scripts.lint.doubleQuote'
  | 'scripts.lint.heredoc';

/** The delimiter after a `<<`, quoted or not: `<<EOF`, `<< 'EOF'`, `<<-EOF`. */
const HEREDOC_DELIMITER = /^-?\s*['"]?(\w+)['"]?/;

/** What the scan carries from one line to the next. Quotes span newlines in the shell. */
interface ScanState {
  inSingle: boolean;
  inDouble: boolean;
  inHeredoc: boolean;
  heredocDelim: string;
}

/**
 * Returns a message key per unterminated construct.
 *
 * Keys rather than sentences so the caller decides the language, which also keeps this testable
 * without a store.
 */
export function basicBashLint(content: string): BashLintProblem[] {
  const state: ScanState = { inSingle: false, inDouble: false, inHeredoc: false, heredocDelim: '' };
  for (const line of content.split('\n')) {
    scanLine(line, state);
  }

  const problems: BashLintProblem[] = [];
  if (state.inSingle) problems.push('scripts.lint.singleQuote');
  if (state.inDouble) problems.push('scripts.lint.doubleQuote');
  if (state.inHeredoc) problems.push('scripts.lint.heredoc');
  return problems;
}

function scanLine(line: string, state: ScanState): void {
  if (state.inHeredoc) {
    // Inside a heredoc, nothing is shell syntax until the delimiter appears on a line of its own.
    if (line.trim() === state.heredocDelim) state.inHeredoc = false;
    return;
  }
  for (let i = 0; i < line.length; i++) {
    // A backslash escapes whatever follows, including a quote, so skip the pair outright.
    if (line[i] === '\\' && i < line.length - 1) i++;
    else i = scanChar(line, i, state);
  }
}

/**
 * Reads one character and returns the index to continue from.
 *
 * The heredoc opener is found here rather than by a separate regex over the line. That is not a
 * tidiness choice: the regex this replaced was anchored to the start of the line, so it only ever
 * matched a line beginning with `<<` — which real shell text never does, since the redirection
 * follows a command. The check had been dead for as long as it existed. Reading it inside the scan
 * is also what makes looking anywhere in the line safe: the quote state is known at this point, so
 * `echo "a << b"` is text rather than an opener.
 */
function scanChar(line: string, i: number, state: ScanState): number {
  const c = line[i];
  const quoted = state.inSingle || state.inDouble;
  if (!quoted && c === "'") state.inSingle = true;
  else if (state.inSingle && c === "'") state.inSingle = false;
  else if (!quoted && c === '"') state.inDouble = true;
  else if (state.inDouble && c === '"') state.inDouble = false;
  else if (!quoted && c === '<' && line[i + 1] === '<' && line[i + 2] !== '<') {
    // Not a herestring (`<<<`), which takes a word rather than opening a block.
    const opened = line.slice(i + 2).match(HEREDOC_DELIMITER);
    if (opened) {
      state.inHeredoc = true;
      state.heredocDelim = opened[1];
    }
  }
  return i;
}
