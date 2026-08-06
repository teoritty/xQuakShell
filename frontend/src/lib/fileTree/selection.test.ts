// frontend/src/lib/fileTree/selection.test.ts
import { selectNode, clearSelection, findNode, type Selection } from './selection';
import type { FileNode } from './types';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error('FAIL: ' + msg);
}
function node(name: string, isDir = false): FileNode {
  return { name, path: '/d/' + name, isDir, size: 0 };
}
function sel(paths: string[], last: string | null = null): Selection {
  return { selectedPaths: new Set(paths), lastSelectedPath: last };
}
const rows = [node('a'), node('b'), node('c'), node('d')];
const paths = (s: Selection) => [...s.selectedPaths].sort();

// --- plain click replaces ---
let s = selectNode(rows, sel(['/d/a', '/d/b']), '/d/c');
assert(paths(s).join() === '/d/c', 'plain click replaces the whole selection');
assert(s.lastSelectedPath === '/d/c', 'plain click moves the anchor');

// --- ctrl/meta toggles ---
s = selectNode(rows, sel(['/d/a']), '/d/b', { ctrlKey: true });
assert(paths(s).join() === '/d/a,/d/b', 'ctrl adds to the selection');
s = selectNode(rows, sel(['/d/a', '/d/b']), '/d/b', { ctrlKey: true });
assert(paths(s).join() === '/d/a', 'ctrl on a selected row removes it');
s = selectNode(rows, sel(['/d/a']), '/d/b', { metaKey: true });
assert(paths(s).join() === '/d/a,/d/b', 'meta behaves as ctrl');

// --- shift extends from the anchor ---
s = selectNode(rows, sel(['/d/b'], '/d/b'), '/d/d', { shiftKey: true });
assert(paths(s).join() === '/d/b,/d/c,/d/d', 'shift extends forwards inclusively');
s = selectNode(rows, sel(['/d/d'], '/d/d'), '/d/b', { shiftKey: true });
assert(paths(s).join() === '/d/b,/d/c,/d/d', 'shift extends backwards inclusively');
s = selectNode(rows, sel(['/d/b'], '/d/b'), '/d/d', { shiftKey: true });
assert(s.lastSelectedPath === '/d/b', 'shift leaves the anchor where it was');

// --- shift with no usable anchor ---
s = selectNode(rows, sel([]), '/d/c', { shiftKey: true });
assert(paths(s).join() === '/d/c', 'shift with no anchor selects just the clicked row');
s = selectNode(rows, sel(['/d/a'], '/d/gone'), '/d/c', { shiftKey: true });
assert(paths(s).join() === '/d/a,/d/c', 'an anchor outside the listing degrades to a single add');

// --- shift on a row outside the current listing ---
// An expanded subdirectory's rows are rendered by the recursive node component
// but are NOT in tree.get(currentPath). The panes indexed the listing without
// checking, so a shift-click on one of those rows walked from index -1 and
// dereferenced nodes[-1].path. This is that crash, pinned.
s = selectNode(rows, sel(['/d/b'], '/d/b'), '/nested/deep/x', { shiftKey: true });
assert(paths(s).join() === '/d/b', 'shift-clicking a row outside the listing changes nothing');

// --- clearSelection ---
const cleared = clearSelection();
assert(cleared.selectedPaths.size === 0 && cleared.lastSelectedPath === null, 'clearSelection empties both halves');

// --- findNode searches every loaded directory ---
const tree = new Map<string, FileNode[]>([
  ['/d', rows],
  ['/other', [node('z')]],
]);
assert(findNode(tree, '/d/c')?.name === 'c', 'findNode finds a row in the current directory');
assert(findNode(tree, '/d/z')?.name === 'z', 'findNode searches other loaded directories too');
assert(findNode(tree, '/nope') === undefined, 'findNode returns undefined for an unknown path');

console.log('OK fileTree/selection');
