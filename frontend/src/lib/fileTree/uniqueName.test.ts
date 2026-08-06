// frontend/src/lib/fileTree/uniqueName.test.ts
import { uniqueName } from './uniqueName';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error('FAIL: ' + msg);
}

// --- no collision ---
assert(uniqueName([], 'New Folder') === 'New Folder', 'an empty directory takes the base name');
assert(uniqueName(['other'], 'New Folder') === 'New Folder', 'an unrelated name does not collide');

// --- the suffix starts at 2, not 1 ---
assert(uniqueName(['New Folder'], 'New Folder') === 'New Folder (2)', 'the second folder is (2)');
assert(uniqueName(['New Folder', 'New Folder (2)'], 'New Folder') === 'New Folder (3)', 'the third is (3)');

// --- gaps are filled, not skipped ---
assert(uniqueName(['New Folder', 'New Folder (3)'], 'New Folder') === 'New Folder (2)', 'the first free number wins');

// --- a name that already carries a suffix is just another name ---
assert(uniqueName(['New Folder (2)'], 'New Folder') === 'New Folder', 'a suffixed name alone does not block the base');

// --- duplicates in the listing do not confuse the count ---
assert(uniqueName(['New Folder', 'New Folder'], 'New Folder') === 'New Folder (2)', 'a repeated name counts once');

// --- works for files too, suffix and all ---
assert(uniqueName(['New File'], 'New File') === 'New File (2)', 'files number the same way');

console.log('OK fileTree/uniqueName');
