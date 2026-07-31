// THE test of this feature.
//
// Discovery rows share the flat row list with folders and connections. If one of
// them ever reached connectionIdsInSelection or folderIdsInSelection, a
// Shift-selection dragged through an expanded subtree would arrive at the tree's
// Delete item carrying real connection ids the user never pointed at — and
// deletions are not undoable. Everything below exists to make that impossible,
// at three independent layers, so no single mistake is enough to cause it.
import type { Connection, Folder } from '../../stores/appState';
import type { DiscoverySnapshot } from '../../api/discovery';
import { buildTree, flattenTree } from './buildTree';
import { computeDropZone, isNodeEditing, isNoOpDragOver, resolveDragPayload, shouldShowDropIndicator } from './dndGuards';
import {
  connectionIdsForDelete,
  connectionIdsInSelection,
  folderIdsInSelection,
  prepareContextMenuSelection,
  selectTreeNode,
  syncSelectionStores,
} from './selection';
import {
  emptyDiscoverySelection,
  isRowSelected,
  moveDiscoverySelection,
  selectDiscoveryRow,
  selectedDiscoveryRows,
} from './discoverySelection';
import { discoveryNodeId, isDiscoveryNodeId } from './types';
import { get, writable } from 'svelte/store';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

const folders: Folder[] = [{ id: 'f1', name: 'Prod', parentId: '', order: 0 }];
const connections: Connection[] = [
  { id: 'c1', name: 'web', host: 'h1', port: 22, folderId: 'f1', order: 0, users: [], defaultUserId: '' },
  { id: 'c2', name: 'db', host: 'h2', port: 22, folderId: 'f1', order: 1, users: [], defaultUserId: '' },
];

const snapshot: DiscoverySnapshot = {
  connectionId: 'c1',
  plugins: [
    {
      pluginId: 'p1',
      nodes: [
        { id: 'containers', parentId: '', kind: 'group', label: 'Containers', order: 0, actions: [] },
        { id: 'vols', parentId: '', kind: 'instance', label: 'Volumes', order: 1, actions: [] },
      ],
      branches: { '': { state: 'ready' } },
    },
  ],
};

const tree = buildTree(folders, connections, new Set(['f1']), '', {
  snapshots: new Map([['c1', snapshot]]),
  expanded: new Map([['c1', new Set([''])]]),
});
const flat = flattenTree(tree);
const discoveryRows = flat.filter((n) => n.type === 'discovery');
const rowOrder = flat.map((n) => `${n.type}:${n.name}`);

assert(discoveryRows.length === 2, `the subtree is actually rendered, got ${discoveryRows.length}`);
assert(
  rowOrder.join(' | ') === 'folder:Prod | connection:web | discovery:Containers | discovery:Volumes | connection:db',
  `the subtree sits between the two connections — the exact shape this test is about: ${rowOrder.join(' | ')}`
);

// --- Layer 1: the ids are structurally in a different namespace ---
for (const rowNode of discoveryRows) {
  assert(isDiscoveryNodeId(rowNode.id), 'every discovery row id carries the prefix');
  assert(
    !connections.some((c) => c.id === rowNode.id) && !folders.some((f) => f.id === rowNode.id),
    'a discovery row id can never equal a connection or folder id'
  );
}

// --- Layer 2: selectTreeNode refuses to put them in the connection selection ---
{
  // The exact gesture: click the connection ABOVE the subtree, then Shift-click
  // the connection BELOW it. The range spans both discovery rows.
  const first = selectTreeNode('c1', flat, null, new Set());
  const spanning = selectTreeNode('c2', flat, first.lastSelectedPath, first.selectedPaths, {
    shiftKey: true,
  } as MouseEvent);
  assert(spanning.selectedPaths.has('c1') && spanning.selectedPaths.has('c2'), 'shift still reaches past the subtree');
  for (const rowNode of discoveryRows) {
    assert(!spanning.selectedPaths.has(rowNode.id), 'a shift range must skip discovery rows, not absorb them');
  }
  assert(spanning.selectedPaths.size === 2, `only the two connections are selected, got ${spanning.selectedPaths.size}`);

  // Clicking a discovery row directly leaves the connection selection alone.
  const afterDiscoveryClick = selectTreeNode(discoveryRows[0].id, flat, 'c2', spanning.selectedPaths);
  assert(
    afterDiscoveryClick.selectedPaths === spanning.selectedPaths,
    'clicking a discovery row does not enter this selection at all'
  );
  // Ctrl-click on a discovery row cannot add it either.
  const afterCtrl = selectTreeNode(discoveryRows[0].id, flat, 'c2', spanning.selectedPaths, {
    ctrlKey: true,
  } as MouseEvent);
  assert(!afterCtrl.selectedPaths.has(discoveryRows[0].id), 'ctrl-click cannot smuggle a discovery row in');
}

// --- Layer 3: even a hand-forged poisoned selection cannot produce deletions ---
{
  // Simulate the failure this test guards against having already happened
  // upstream: discovery ids sitting in selectedPaths. Nothing downstream may act
  // on them.
  const poisoned = new Set([
    'c1',
    ...discoveryRows.map((n) => n.id),
    discoveryNodeId('c1', 'p1', 'c2'), // a plugin node whose id spells a real connection
  ]);
  // Reaching this state at all is a broken invariant, so the filter is expected
  // to say so rather than clean up quietly. Capturing console.warn is what makes
  // an otherwise-unfalsifiable backstop testable.
  const warnings: string[] = [];
  const realWarn = console.warn;
  console.warn = (...args: unknown[]) => warnings.push(args.join(' '));

  const connIds = connectionIdsInSelection(poisoned, connections);
  assert(connIds.join(',') === 'c1', `only the genuine connection survives, got "${connIds.join(',')}"`);
  assert(warnings.length === 1, `the dropped ids are reported, got ${warnings.length} warning(s)`);
  assert(
    warnings[0].includes('discovery node id') && warnings[0].includes('bug upstream'),
    `the warning names the problem, got "${warnings[0]}"`
  );
  assert(folderIdsInSelection(poisoned, folders).length === 0, 'no folder is conjured out of discovery ids');

  warnings.length = 0;
  connectionIdsInSelection(new Set(['c1', 'c2']), connections);
  assert(warnings.length === 0, 'a clean selection is silent — the warning is a signal, not noise');

  const toDelete = connectionIdsForDelete('c1', poisoned, connections);
  assert(toDelete.join(',') === 'c1', `delete acts on exactly one connection, got "${toDelete.join(',')}"`);
  assert(
    connectionIdsForDelete(discoveryRows[0].id, poisoned, connections).length === 0,
    'a delete aimed at a discovery row deletes nothing'
  );

  const stores = {
    selectedConnectionId: writable(''),
    selectedConnectionIds: writable(new Set<string>()),
    selectedFolderId: writable(''),
  };
  syncSelectionStores(poisoned, connections, folders, stores);
  console.warn = realWarn;
  assert(get(stores.selectedConnectionIds).size === 1, 'the store never sees a discovery id');
  assert(get(stores.selectedFolderId) === '', 'nor does the folder store');
}

// --- the context menu never solo-selects a discovery row into this selection ---
assert(
  prepareContextMenuSelection(discoveryRows[0], new Set()) === null,
  'right-click on a discovery row does not touch the connection selection'
);

// --- drag and drop: discovery rows neither drag nor accept a drop ---
{
  const payload = { folderIds: ['f1'], connectionIds: ['c1'] };
  for (const rowNode of discoveryRows) {
    assert(
      computeDropZone({} as DragEvent, rowNode) === null,
      'computeDropZone returns null so no insertion indicator is ever drawn over a subtree'
    );
    assert(isNoOpDragOver(payload, rowNode), 'a discovery row is always a no-op drag target');
    assert(
      !shouldShowDropIndicator(payload, rowNode, null, connections, folders, flat),
      'and therefore shows no indicator'
    );
    const dragged = resolveDragPayload(rowNode, new Set(['c1', 'c2', rowNode.id]), new Set(), new Set(['c1', 'c2']));
    assert(
      dragged.folderIds.length === 0 && dragged.connectionIds.length === 0,
      'dragging a discovery row must not smuggle the current connection selection along'
    );
    assert(!isNodeEditing(rowNode, rowNode.id, rowNode.id), 'a discovery row is never in rename mode');
  }
}

// --- a collapsed connection produces no discovery rows at all ---
{
  const collapsed = flattenTree(
    buildTree(folders, connections, new Set(['f1']), '', {
      snapshots: new Map([['c1', snapshot]]),
      expanded: new Map(),
    })
  );
  assert(collapsed.every((n) => n.type !== 'discovery'), 'nothing is observed, so nothing is drawn');
  assert(collapsed.length === 3, 'and the tree is exactly what it was before this feature');
}

// --- two connections, one plugin, the SAME node id: nothing may cross over ---
//
// The ordinary case, not a contrived one: one Docker-ish plugin publishing
// `containers` on both hosts. A discoveryKey is (pluginId, nodeId) and carries no
// connection, so every membership test, every highlight and every DOM lookup has
// to pair it with the connectionId. Comparing keys alone put focus, Enter and the
// context menu into the wrong connection's subtree.
{
  const shared: DiscoverySnapshot['plugins'] = [
    {
      pluginId: 'p1',
      nodes: [{ id: 'containers', parentId: '', kind: 'instance', label: 'Containers', order: 0, actions: [] }],
      branches: { '': { state: 'ready' } },
    },
  ];
  const bothFlat = flattenTree(
    buildTree(folders, connections, new Set(['f1']), '', {
      snapshots: new Map([
        ['c1', { connectionId: 'c1', plugins: shared }],
        ['c2', { connectionId: 'c2', plugins: shared }],
      ]),
      expanded: new Map([
        ['c1', new Set([''])],
        ['c2', new Set([''])],
      ]),
    })
  );
  const both = bothFlat.filter((n) => n.discovery).map((n) => n.discovery!);
  assert(both.length === 2, `both connections draw the row, got ${both.length}`);
  const [underC1, underC2] = both[0].connectionId === 'c1' ? [both[0], both[1]] : [both[1], both[0]];

  // The key is deliberately the same — that is the whole point.
  assert(underC1.key === underC2.key, 'the same plugin node under two hosts shares a discoveryKey');
  // The DOM addressing target is not.
  const idC1 = discoveryNodeId(underC1.connectionId, underC1.pluginId, underC1.nodeId);
  const idC2 = discoveryNodeId(underC2.connectionId, underC2.pluginId, underC2.nodeId);
  assert(idC1 !== idC2, 'TreeNode ids stay distinct, so a focus lookup can tell the rows apart');
  const domIds = bothFlat.filter((n) => n.discovery).map((n) => n.id);
  assert(new Set(domIds).size === 2, 'and the rendered rows carry those distinct ids');

  // A selection living under c2 must not make c1's row look selected...
  const selInC2 = selectDiscoveryRow(emptyDiscoverySelection(), underC2, both);
  assert(isRowSelected(selInC2, underC2), 'the row that was clicked is selected');
  assert(!isRowSelected(selInC2, underC1), 'its twin under the other connection is NOT');
  assert(
    selInC2.keys.has(underC1.key),
    'and the bare key test would have said yes — which is exactly why isRowSelected exists'
  );

  // ...nor may it be acted on. This is the set Enter and the context menu use.
  const acted = selectedDiscoveryRows(selInC2, both);
  assert(acted.length === 1 && acted[0].connectionId === 'c2', 'only the selected connection is acted on');

  // Arrow movement stays inside the connection too.
  const moved = moveDiscoverySelection(selInC2, both, -1, false);
  assert(
    moved.connectionId === 'c2' && moved.keys.size === 1,
    'an arrow at the edge of one subtree does not step into another connection'
  );
}

// --- buildTree without the discovery argument behaves exactly as it always did ---
{
  const legacy = flattenTree(buildTree(folders, connections, new Set(['f1']), ''));
  assert(legacy.length === 3 && legacy.every((n) => n.type !== 'discovery'), 'discovery is opt-in');
}

console.log('discoveryIsolation.test.ts: all passed');
