import type { Connection, Folder } from '../../stores/appState';
import {
  allConnectionsInFolder,
  isFolderAncestor,
  isNoOpDropOnFolder,
  isNoOpDropOnRoot,
  isNoOpReorder,
  isNodeEditing,
  resolveDragPayload,
} from './dndGuards';
import type { TreeNode } from './types';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

const folders: Folder[] = [
  { id: 'f1', name: 'A', parentId: '', order: 0 },
  { id: 'f2', name: 'B', parentId: 'f1', order: 0 },
];
const connections: Connection[] = [
  { id: 'c1', name: 'C1', host: '', port: 22, folderId: 'f1', order: 0, users: [], defaultUserId: '' },
  { id: 'c2', name: 'C2', host: '', port: 22, folderId: 'f2', order: 0, users: [], defaultUserId: '' },
];

assert(isNodeEditing({ type: 'folder', id: 'f1', name: '', depth: 0, parentId: '' }, 'f1', null), 'folder editing');
assert(!isNodeEditing({ type: 'connection', id: 'c1', name: '', depth: 0, parentId: '' }, null, 'c2'), 'conn not editing');

const selectedPaths = new Set(['c1', 'c2']);
const selectedFolderIds = new Set<string>();
const selectedConnectionIds = new Set(['c1', 'c2']);
const payloadMulti = resolveDragPayload(
  { type: 'connection', id: 'c1', name: '', depth: 0, parentId: '' },
  selectedPaths,
  selectedFolderIds,
  selectedConnectionIds
);
assert(payloadMulti.folderIds.length === 0 && payloadMulti.connectionIds.length === 2, 'multi drag payload');

assert(
  isNoOpDropOnFolder({ folderIds: ['f1'], connectionIds: [] }, 'f1', connections, folders),
  'folder onto self'
);
assert(
  isNoOpDropOnFolder({ folderIds: [], connectionIds: ['c1'] }, 'f1', connections, folders),
  'conn already in folder'
);
assert(
  !isNoOpDropOnFolder({ folderIds: [], connectionIds: ['c2'] }, 'f1', connections, folders),
  'conn move needed'
);
assert(isFolderAncestor(folders, 'f1', 'f2'), 'ancestor check');
assert(allConnectionsInFolder(['c1'], connections, 'f1'), 'all in folder');

const flat: TreeNode[] = [{ type: 'connection', id: 'c1', name: '', depth: 1, parentId: 'f1' }];
assert(
  isNoOpReorder({ folderIds: [], connectionIds: ['c1'] }, flat[0], flat, 'before'),
  'reorder self'
);

assert(
  isNoOpDropOnRoot({ folderIds: ['f1'], connectionIds: [] }, connections, folders),
  'folder already at root'
);

const mixedPayload = resolveDragPayload(
  { type: 'folder', id: 'f1', name: '', depth: 0, parentId: '' },
  new Set(['f1', 'c1']),
  new Set(['f1']),
  new Set(['c1'])
);
assert(
  mixedPayload.folderIds.length === 1 && mixedPayload.connectionIds.length === 1,
  'mixed multi drag payload'
);

const flatCross: TreeNode[] = [
  { type: 'connection', id: 'c1', name: '', depth: 1, parentId: 'f1' },
  { type: 'connection', id: 'c3', name: '', depth: 1, parentId: 'f1' },
];
assert(
  !isNoOpReorder({ folderIds: [], connectionIds: ['c2'] }, flatCross[0], flatCross, 'before'),
  'cross-parent reorder is not a no-op'
);

console.log('dndGuards.test.ts: all passed');
