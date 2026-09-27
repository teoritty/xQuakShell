import { compileQuery, findInText, type SearchOptions } from './matcher';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

const plain: SearchOptions = { caseSensitive: false, wholeWord: false, regex: false };

function matches(text: string, query: string, options: Partial<SearchOptions> = {}): string[] {
  const compiled = compileQuery(query, { ...plain, ...options });
  if (!compiled || !compiled.ok) throw new Error(`query ${query} did not compile`);
  return findInText(text, compiled.re, 100).map((r) => text.slice(r.start, r.end));
}

assert(compileQuery('', plain) === null, 'an empty query compiles to nothing, not to match-everything');

// Plain text is literal: regex metacharacters typed without the regex toggle mean themselves.
assert(matches('cost is $5.00 (net)', '$5.00 (').length === 1, 'metacharacters are escaped when regex is off');
assert(matches('a.b axb', 'a.b').join() === 'a.b', `a dot is a dot without regex, got ${matches('a.b axb', 'a.b')}`);

// Case.
assert(matches('Error error ERROR', 'error').length === 3, 'case-insensitive by default');
assert(matches('Error error ERROR', 'error', { caseSensitive: true }).join() === 'error',
  'match case finds only the exact spelling');
assert(matches('Ошибка ОШИБКА', 'ошибка').length === 2, 'case folding works for Cyrillic too');

// Whole word, including the non-ASCII case JavaScript's \b gets wrong.
assert(matches('cat concat cat_x cat.', 'cat', { wholeWord: true }).length === 2,
  `whole word skips concat and cat_x, got ${matches('cat concat cat_x cat.', 'cat', { wholeWord: true })}`);
assert(matches('кот котик кот', 'кот', { wholeWord: true }).length === 2,
  'whole word treats Cyrillic letters as word characters');

// Regex.
assert(matches('pid 12 pid 345', 'pid \\d+', { regex: true }).join('|') === 'pid 12|pid 345', 'regex is honoured');
const bad = compileQuery('(unclosed', { ...plain, regex: true });
assert(!!bad && !bad.ok && bad.error.length > 0, 'an invalid regex reports why instead of throwing');

// A pattern that can match nothing must not loop, and must still find its non-empty matches.
assert(matches('aaa b aa', 'a*', { regex: true }).join('|') === 'aaa|aa', 'empty matches are skipped, not looped on');
assert(matches('😀x😀', '(?=x)', { regex: true }).length === 0, 'a lookahead alone matches nothing visible');

// The limit caps the work on a pathological line.
{
  const compiled = compileQuery('a', plain);
  if (!compiled || !compiled.ok) throw new Error('compile');
  assert(findInText('a'.repeat(50), compiled.re, 10).length === 10, 'findInText stops at its limit');
}

console.log('matcher.test passed');
