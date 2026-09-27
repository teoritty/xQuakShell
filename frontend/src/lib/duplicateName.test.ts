import { copyBaseName, duplicateName, type CopyNameTemplates } from './duplicateName';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

const en: CopyNameTemplates = { first: '{name} - copy', numbered: '{name} - copy {number}' };
const ru: CopyNameTemplates = { first: '{name} - копия', numbered: '{name} - копия {number}' };

assert(duplicateName('web', new Set(['web']), en) === 'web - copy', 'the first copy is "<name> - copy"');
assert(duplicateName('web', new Set(['web']), ru) === 'web - копия', 'the suffix follows the interface language');

// Taken names are skipped with a number, as a file manager does.
assert(duplicateName('web', new Set(['web', 'web - copy']), en) === 'web - copy 2', 'a taken copy name moves to 2');
assert(duplicateName('web', new Set(['web - copy', 'web - copy 2', 'web - copy 3']), en) === 'web - copy 4', 'numbering skips every taken one');

// Copying a copy numbers it rather than stacking suffixes.
assert(duplicateName('web - copy', new Set(['web', 'web - copy']), en) === 'web - copy 2', 'a copy of a copy is numbered');
assert(duplicateName('web - copy 2', new Set(['web - copy', 'web - copy 2']), en) === 'web - copy 3', 'a numbered copy counts on');
assert(copyBaseName('web - copy 7', en) === 'web', 'the base of a numbered copy');
assert(copyBaseName('web', en) === 'web', 'a name that is not a copy is its own base');

// Characters that mean something in a pattern are ordinary text in a name or a translation.
assert(duplicateName('db (prod) [eu]', new Set(), en) === 'db (prod) [eu] - copy', 'names with regex metacharacters work');
assert(copyBaseName('a.b - copy', en) === 'a.b', 'a dot in a name is a dot');
const odd: CopyNameTemplates = { first: '{name} (copy?)', numbered: '{name} (copy? {number})' };
assert(copyBaseName('x (copy? 3)', odd) === 'x', 'metacharacters in a translation are literal');

// A name that only looks like a copy in another language is left alone.
assert(duplicateName('web - копия', new Set(), en) === 'web - копия - copy', 'another language\'s suffix is just text');

console.log('duplicateName.test passed');
