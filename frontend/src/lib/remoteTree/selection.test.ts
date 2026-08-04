// Regression tests for four reported tree defects, all of which came from the
// same two confusions: "where do new items go" being smuggled through a store
// that anything could write, and delete resolving folders by a different rule
// than connections.
import type { Connection, Folder } from '../../stores/appState';
import {
  creationTargetFolderId,
  deleteTargets,
  selectionDeleteTargets,
  shouldClearTreeSelection,
  syncSelectionStores,
} from './selection';
import { describeDeleteTargets } from './deletePrompt';
import { discoveryNodeId } from './types';
import { get, writable } from 'svelte/store';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

// root
// ├── prod           (f1)
// │   ├── eu         (f2)
// │   │   └── db     (c3)
// │   └── web        (c1)
// ├── staging        (f3)
// └── local          (c2)
const folders: Folder[] = [
  { id: 'f1', name: 'prod', parentId: '', order: 0 },
  { id: 'f2', name: 'eu', parentId: 'f1', order: 0 },
  { id: 'f3', name: 'staging', parentId: '', order: 1 },
];
const connections: Connection[] = [
  { id: 'c1', name: 'web', host: 'h1', port: 22, folderId: 'f1', order: 0, users: [], defaultUserId: '' },
  { id: 'c2', name: 'local', host: 'h2', port: 22, folderId: '', order: 1, users: [], defaultUserId: '' },
  { id: 'c3', name: 'db', host: 'h3', port: 22, folderId: 'f2', order: 0, users: [], defaultUserId: '' },
];

const target = (...ids: string[]) => creationTargetFolderId(new Set(ids), connections, folders);

// --- creationTargetFolderId: where a new folder or connection goes ----------

assert(target() === '', 'nothing selected creates at the root');
assert(target('f1') === 'f1', 'a selected folder creates inside itself');

// Bug 4: clicking a connection once used to send the next "New connection" to
// the root, even though the highlighted row lived inside a folder.
assert(target('c1') === 'f1', 'a selected connection creates next to itself, in its own folder');
assert(target('c3') === 'f2', 'that holds at any depth');
assert(target('c2') === '', 'a connection already at the root creates at the root');
assert(target('c1', 'c3') === '', 'connections spread over different folders have no single answer');
assert(target('c1', 'c1') === 'f1', 'the same connection twice is still one folder');
assert(target('f1', 'f3') === '', 'two folders have no single answer either');
assert(target('f1', 'c1') === '', 'a mixed selection has no single answer');
assert(target(discoveryNodeId('c1', 'p1', 'n1')) === '', 'a discovery row is not a place to create anything');

// The store is a projection of the selection and nothing else — this is what
// makes it impossible for a create action to move the target (bug 1).
{
  const stores = {
    selectedConnectionId: writable(''),
    selectedConnectionIds: writable(new Set<string>()),
    creationTargetFolderId: writable('stale'),
  };
  syncSelectionStores(new Set(['c3']), connections, folders, stores);
  assert(get(stores.creationTargetFolderId) === 'f2', 'syncSelectionStores derives the creation target from the selection');
}

// --- shouldClearTreeSelection: the toolbar is not "empty space" -------------

function fakeTarget(className: string): Element {
  // Only `closest` is consulted, and only for the ancestor-or-self case that
  // matters here; a full DOM would add a dependency for two string compares.
  return {
    closest(selector: string) {
      return selector.split(',').some((s) => s.trim() === `.${className}`) ? this : null;
    },
  } as unknown as Element;
}

assert(shouldClearTreeSelection(fakeTarget('tree-empty')), 'a click on empty space clears the selection');
assert(!shouldClearTreeSelection(null), 'a click with no target clears nothing');
assert(!shouldClearTreeSelection(fakeTarget('tree-node')), 'a click on a row does not clear the selection');
assert(!shouldClearTreeSelection(fakeTarget('context-menu')), 'nor does a click in the context menu');
assert(!shouldClearTreeSelection(fakeTarget('import-menu')), 'nor does a click in the import menu');
// Bug 2: the toolbar's own click used to reach the window handler and wipe the
// selection, so "New connection" hit the selected folder once and the root
// after that.
assert(
  !shouldClearTreeSelection(fakeTarget('tree-toolbar')),
  'a toolbar button acts ON the selection and must not destroy it'
);

// --- deleteTargets: one rule for folders and connections -------------------

{
  const single = deleteTargets('f1', new Set(['f1']), connections, folders);
  assert(single.folderIds.join(',') === 'f1' && single.connectionIds.length === 0, 'a lone folder deletes itself');
}
{
  const single = deleteTargets('c1', new Set(['c1']), connections, folders);
  assert(single.connectionIds.join(',') === 'c1' && single.folderIds.length === 0, 'a lone connection deletes itself');
}

// Bug 3: three selected folders used to delete one.
{
  const many = deleteTargets('f1', new Set(['f1', 'f3']), connections, folders);
  assert(many.folderIds.sort().join(',') === 'f1,f3', 'delete on a selected folder takes every selected folder');
}
{
  const mixed = deleteTargets('c2', new Set(['f3', 'c2']), connections, folders);
  assert(
    mixed.folderIds.join(',') === 'f3' && mixed.connectionIds.join(',') === 'c2',
    'a mixed selection deletes both kinds'
  );
}

// Right-clicking outside the selection acts on that row alone — the menu has
// already solo-selected it (prepareContextMenuSelection).
{
  const outside = deleteTargets('f3', new Set(['f1', 'c1']), connections, folders);
  assert(
    outside.folderIds.join(',') === 'f3' && outside.connectionIds.length === 0,
    'delete aimed outside the selection acts on that row alone'
  );
}

// The backend cascade already removes the subtree; naming it twice would fail
// the second call on an id that no longer exists.
{
  const nested = selectionDeleteTargets(new Set(['f1', 'f2', 'c1', 'c3', 'c2']), connections, folders);
  assert(nested.folderIds.join(',') === 'f1', 'a nested folder is dropped — its parent takes it with it');
  assert(
    nested.connectionIds.join(',') === 'c2',
    `connections inside a doomed folder are dropped, got "${nested.connectionIds.join(',')}"`
  );
}

// --- describeDeleteTargets: the dialog states the real blast radius --------

{
  const p = describeDeleteTargets({ folderIds: [], connectionIds: ['c1'] }, folders, connections);
  assert(p.title === 'Delete Connection' && !p.critical, 'deleting one connection is an ordinary confirm');
  assert(p.message.includes('"web"'), 'and it names the connection');
}
{
  const p = describeDeleteTargets({ folderIds: ['f3'], connectionIds: [] }, folders, connections);
  assert(p.title === 'Delete Folder' && !p.critical, 'deleting an empty folder is an ordinary confirm');
}
{
  const p = describeDeleteTargets({ folderIds: ['f1'], connectionIds: [] }, folders, connections);
  assert(p.critical, 'deleting a folder with connections in it is critical');
  assert(p.message.includes('2 connection(s)'), `it counts the whole subtree, got "${p.message}"`);
}
{
  const p = describeDeleteTargets({ folderIds: ['f1', 'f3'], connectionIds: ['c2'] }, folders, connections);
  assert(p.title === 'Delete Multiple Items' && p.critical, 'a mixed batch is critical');
  assert(
    p.message.includes('2 folder(s) and 1 connection(s)') && p.message.includes('including 2 connection(s)'),
    `it spells out both counts and the cascade, got "${p.message}"`
  );
}

console.log('selection.test passed');
