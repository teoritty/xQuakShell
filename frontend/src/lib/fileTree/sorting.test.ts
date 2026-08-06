// frontend/src/lib/fileTree/sorting.test.ts
import { formatSize, parseTimestamp, compareValues, sortValue, applySort, sortTree } from './sorting';
import type { FileNode, SortState } from './types';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error('FAIL: ' + msg);
}
function node(name: string, over: Partial<FileNode> = {}): FileNode {
  return { name, path: '/' + name, isDir: false, size: 0, ...over };
}
const off: SortState = { sortEnabled: false, sortKey: null, sortDir: 'asc' };
const by = (key: SortState['sortKey'], dir: SortState['sortDir'] = 'asc'): SortState => ({
  sortEnabled: true,
  sortKey: key,
  sortDir: dir,
});
const names = (nodes: FileNode[]) => nodes.map((n) => n.name).join(',');

// --- formatSize ---
assert(formatSize(0) === '0 B', 'zero bytes');
assert(formatSize(1023) === '1023 B', 'below 1 KiB stays in bytes');
assert(formatSize(1024) === '1.0 KB', 'the KB step is at 1024');
assert(formatSize(1048576) === '1.0 MB', 'the MB step is at 1024 KiB');
assert(formatSize(1073741824) === '1.0 GB', 'the GB step is at 1024 MiB');
assert(formatSize(1610612736) === '1.5 GB', 'one decimal place above the step');

// --- parseTimestamp: unparseable sorts oldest, never NaN ---
assert(parseTimestamp(undefined) === -1, 'missing timestamp sorts oldest');
assert(parseTimestamp('') === -1, 'empty timestamp sorts oldest');
assert(parseTimestamp('not a date') === -1, 'unparseable timestamp sorts oldest, not NaN');
assert(parseTimestamp('2020-01-01T00:00:00Z') === Date.parse('2020-01-01T00:00:00Z'), 'a real date parses');

// --- compareValues ---
assert(compareValues(1, 2) < 0, 'numbers compare numerically');
assert(compareValues('a', 'b') < 0, 'strings compare lexically');
assert(compareValues(10, 9) > 0, 'numbers do not compare as strings');

// --- sortValue: the owner column falls back to the group ---
assert(sortValue(node('A'), 'name') === 'a', 'name sorts case-insensitively');
assert(sortValue(node('x', { size: 42 }), 'size') === 42, 'size sorts by its number');
assert(sortValue(node('x', { owner: 'Root', group: 'wheel' }), 'owner') === 'root', 'owner wins when present');
assert(sortValue(node('x', { group: 'Wheel' }), 'owner') === 'wheel', 'group is the fallback for a remote node');
assert(sortValue(node('x'), 'owner') === '', 'a local node with neither sorts as empty');

// --- applySort: directories first, always ---
const mixed = [node('zzz', { isDir: true }), node('aaa'), node('mmm', { isDir: true })];
assert(names(applySort(mixed, off)) === 'mmm,zzz,aaa', 'directories come first even with sorting off');
assert(names(applySort(mixed, by('name', 'desc'))) === 'zzz,mmm,aaa', 'descending never mixes files into directories');

// --- applySort: name is the tiebreak, so the order is total ---
const sameSize = [node('b', { size: 5 }), node('a', { size: 5 }), node('c', { size: 5 })];
assert(names(applySort(sameSize, by('size'))) === 'a,b,c', 'equal keys fall back to name');
assert(names(applySort(sameSize, by('size', 'desc'))) === 'a,b,c', 'the name tiebreak is not reversed by direction');

// --- applySort does not mutate its input ---
const original = [node('b'), node('a')];
const before = names(original);
applySort(original, by('name'));
assert(names(original) === before, 'applySort leaves the input array alone');

// --- applySort tolerates a missing listing ---
assert(applySort(undefined as unknown as FileNode[], off).length === 0, 'a missing listing sorts to empty');

// --- sortTree ---
const raw = new Map<string, FileNode[]>([
  ['/', [node('b'), node('a')]],
  ['/sub', [node('d'), node('c')]],
]);
const sorted = sortTree(raw, by('name'));
assert(names(sorted.get('/')!) === 'a,b' && names(sorted.get('/sub')!) === 'c,d', 'sortTree sorts every listing');
assert(raw.get('/')![0].name === 'b', 'sortTree leaves the raw tree untouched');
const restored = sortTree(raw, off);
assert(names(restored.get('/')!) === 'b,a', 'sorting off restores the listing order the server gave');
assert(restored !== raw, 'sorting off still returns a new map, so Svelte sees the change');

console.log('OK fileTree/sorting');
