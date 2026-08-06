// frontend/src/lib/fileTree/deletePrompt.test.ts
import { describeDelete } from './deletePrompt';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error('FAIL: ' + msg);
}

// --- one empty file or directory: the light prompt ---
let p = describeDelete({ pathsToDelete: [], name: 'notes.txt', childCount: 0 });
assert(p.title === 'Delete?', 'a single target gets the short title');
assert(p.message === 'Delete "notes.txt"?', 'a single target is named in the message');
assert(!p.critical, 'a single empty target is not critical');
assert(!p.requireCheckbox, 'a single empty target needs no checkbox');

// --- one directory with contents: escalates ---
p = describeDelete({ pathsToDelete: [], name: 'src', childCount: 12 });
assert(p.title === 'Delete items?', 'a non-empty directory gets the plural title');
assert(p.message.includes('"src"') && p.message.includes('12 item(s) inside'), 'the child count is stated');
assert(p.critical && p.requireCheckbox, 'a non-empty directory requires the checkbox');

// --- a multi-selection: escalates ---
p = describeDelete({ pathsToDelete: ['/a', '/b', '/c'], name: '', childCount: 3 });
assert(p.message === 'You are deleting 3 item(s). This action cannot be undone.', 'a multi-selection is counted');
assert(p.critical && p.requireCheckbox, 'a multi-selection requires the checkbox');

// --- exactly one path in the list: still the light prompt ---
// Deleting one row via a selection of one must not be scarier than deleting the
// same row from the context menu.
p = describeDelete({ pathsToDelete: ['/a'], name: '', childCount: 0 });
assert(p.title === 'Delete?', 'a one-item selection keeps the short title');
assert(!p.critical && !p.requireCheckbox, 'a one-item selection needs no checkbox');
assert(p.message === 'You are deleting 1 item(s). This action cannot be undone.', 'it still reports the count');

// --- the checkbox and the critical styling never disagree ---
for (const t of [
  { pathsToDelete: [], name: 'x', childCount: 0 },
  { pathsToDelete: [], name: 'x', childCount: 1 },
  { pathsToDelete: ['/a'], name: '', childCount: 0 },
  { pathsToDelete: ['/a', '/b'], name: '', childCount: 2 },
]) {
  const d = describeDelete(t);
  assert(d.critical === d.requireCheckbox, 'critical and requireCheckbox stay in step');
}

console.log('OK fileTree/deletePrompt');
