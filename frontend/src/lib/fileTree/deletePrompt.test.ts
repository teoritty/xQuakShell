// frontend/src/lib/fileTree/deletePrompt.test.ts
import { describeDelete } from './deletePrompt';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error('FAIL: ' + msg);
}

// The wording lives in the language packs, so the lookup here echoes the key and its variables.
// What is asserted is which message appears and which counts reach it - a reworded translation
// must not fail this, and a message that stops naming its target must.
const label = (key: string, vars?: Record<string, string | number>) =>
  vars ? `${key}(${Object.entries(vars).map(([k, v]) => `${k}=${v}`).join(',')})` : key;

// --- one empty file or directory: the light prompt ---
let p = describeDelete({ pathsToDelete: [], name: 'notes.txt', childCount: 0 }, label);
assert(p.title === 'files.delete.one.title', 'a single target gets the short title');
assert(p.message.includes('name=notes.txt'), 'a single target is named in the message');
assert(!p.critical, 'a single empty target is not critical');
assert(!p.requireCheckbox, 'a single empty target needs no checkbox');

// --- one directory with contents: escalates ---
p = describeDelete({ pathsToDelete: [], name: 'src', childCount: 12 }, label);
assert(p.title === 'files.delete.many.title', 'a non-empty directory gets the plural title');
assert(p.message.includes('name=src') && p.message.includes('count=12'), 'the child count is stated');
assert(p.critical && p.requireCheckbox, 'a non-empty directory requires the checkbox');
assert(
  p.message.startsWith('security.'),
  'the warning before an irreversible recursive delete is a security string a disk pack must not reword',
);

// --- a multi-selection: escalates ---
p = describeDelete({ pathsToDelete: ['/a', '/b', '/c'], name: '', childCount: 3 }, label);
assert(p.message === 'security.files.delete.many(count=3)', 'a multi-selection is counted');
assert(p.critical && p.requireCheckbox, 'a multi-selection requires the checkbox');

// --- exactly one path in the list: still the light prompt ---
// Deleting one row via a selection of one must not be scarier than deleting the
// same row from the context menu.
p = describeDelete({ pathsToDelete: ['/a'], name: '', childCount: 0 }, label);
assert(p.title === 'files.delete.one.title', 'a one-item selection keeps the short title');
assert(!p.critical && !p.requireCheckbox, 'a one-item selection needs no checkbox');
assert(p.message === 'security.files.delete.many(count=1)', 'it still reports the count');

// --- the checkbox and the critical styling never disagree ---
for (const t of [
  { pathsToDelete: [], name: 'x', childCount: 0 },
  { pathsToDelete: [], name: 'x', childCount: 1 },
  { pathsToDelete: ['/a'], name: '', childCount: 0 },
  { pathsToDelete: ['/a', '/b'], name: '', childCount: 2 },
]) {
  const d = describeDelete(t, label);
  assert(d.critical === d.requireCheckbox, 'critical and requireCheckbox stay in step');
}

console.log('OK fileTree/deletePrompt');
