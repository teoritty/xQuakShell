// frontend/src/lib/fileTree/selection.test.ts
import { selectNode, clearSelection, findNode, visibleRows, type Selection } from './selection';
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

// --- visibleRows: the screen order shift ranges over ---
// /r holds dir x (expanded), file y, dir z (collapsed); x holds dir x/p
// (expanded) and file x/q; x/p holds file x/p/k. On screen that reads
// x, x/p, x/p/k, x/q, y, z — z's loaded listing must not appear.
function n(path: string, isDir = false): FileNode {
  return { name: path.split('/').pop()!, path, isDir, size: 0 };
}
const nested = new Map<string, FileNode[]>([
  ['/r', [n('/r/x', true), n('/r/y'), n('/r/z', true), n('/r/.h')]],
  ['/r/x', [n('/r/x/p', true), n('/r/x/q')]],
  ['/r/x/p', [n('/r/x/p/k')]],
  ['/r/z', [n('/r/z/hidden-by-collapse')]],
]);
const open = new Set(['/r/x', '/r/x/p']);
const shown = visibleRows(nested, '/r', open, (r) => !r.name.startsWith('.'));
assert(
  shown.map((r) => r.path).join() === '/r/x,/r/x/p,/r/x/p/k,/r/x/q,/r/y,/r/z',
  'visibleRows splices expanded children under their row, skips collapsed ones and filtered top rows',
);
assert(visibleRows(nested, '/r', open).length === 7, 'visibleRows keeps every top row without a filter');

// The reported bug: inside an expanded folder shift-click did nothing.
s = selectNode(shown, sel(['/r/x/p'], '/r/x/p'), '/r/x/q', { shiftKey: true });
assert(paths(s).join() === '/r/x/p,/r/x/p/k,/r/x/q', 'shift extends between rows of an expanded folder');
s = selectNode(shown, sel(['/r/x/p/k'], '/r/x/p/k'), '/r/y', { shiftKey: true });
assert(paths(s).join() === '/r/x/p/k,/r/x/q,/r/y', 'shift extends from a nested row out to a root row');

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
