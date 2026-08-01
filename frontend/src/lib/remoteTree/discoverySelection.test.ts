import {
  emptyDiscoverySelection,
  moveDiscoverySelection,
  pruneDiscoverySelection,
  selectDiscoveryRow,
  selectedDiscoveryRows,
} from './discoverySelection';
import { discoveryKey, type DiscoveryRow } from './types';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

function row(pluginId: string, nodeId: string, parentNodeId = ''): DiscoveryRow {
  return {
    connectionId: 'conn1',
    pluginId,
    nodeId,
    key: discoveryKey(pluginId, nodeId),
    parentKey: discoveryKey(pluginId, parentNodeId),
    kind: 'instance',
    label: nodeId,
    iconId: '',
    status: null,
    actions: [],
    defaultActionId: '',
    branchState: 'ready',
    stale: false,
    actionsBlocked: false,
    expanded: false,
  };
}

// Two sibling groups under one plugin, plus children of one of them, plus a
// second plugin's root-level row.
const a = row('p1', 'a');
const b = row('p1', 'b');
const c = row('p1', 'c');
const childX = row('p1', 'x', 'a');
const childY = row('p1', 'y', 'a');
const otherPlugin = row('p2', 'a');
const visible = [a, b, c, childX, childY, otherPlugin];

// --- plain click replaces ---
{
  const sel = selectDiscoveryRow(emptyDiscoverySelection(), b, visible);
  assert(sel.keys.size === 1 && sel.keys.has(b.key), 'plain click selects exactly one row');
  assert(sel.pluginId === 'p1' && sel.parentKey === b.parentKey, 'selection records plugin and parent');
}

// --- Ctrl adds a sibling ---
{
  let sel = selectDiscoveryRow(emptyDiscoverySelection(), a, visible);
  sel = selectDiscoveryRow(sel, c, visible, { ctrlKey: true });
  assert(sel.keys.size === 2 && sel.keys.has(a.key) && sel.keys.has(c.key), 'ctrl adds a sibling');
  sel = selectDiscoveryRow(sel, c, visible, { ctrlKey: true });
  assert(sel.keys.size === 1, 'ctrl toggles a selected row back off');
}

// --- ONE PARENT: Ctrl across parents collapses to a plain click ---
{
  let sel = selectDiscoveryRow(emptyDiscoverySelection(), a, visible);
  sel = selectDiscoveryRow(sel, childX, visible, { ctrlKey: true });
  assert(sel.keys.size === 1 && sel.keys.has(childX.key), 'ctrl cannot cross a parent boundary');
  assert(sel.parentKey === childX.parentKey, 'the selection moved to the new parent wholesale');
}

// --- ONE PARENT (and therefore one plugin): across plugins at the root level ---
{
  let sel = selectDiscoveryRow(emptyDiscoverySelection(), a, visible);
  sel = selectDiscoveryRow(sel, otherPlugin, visible, { ctrlKey: true });
  assert(
    sel.keys.size === 1 && sel.pluginId === 'p2',
    'two plugins at the root level are still different parents — an action needs one pluginId'
  );
}

// --- Shift extends within the parent only ---
{
  let sel = selectDiscoveryRow(emptyDiscoverySelection(), a, visible);
  sel = selectDiscoveryRow(sel, c, visible, { shiftKey: true });
  assert(sel.keys.size === 3, `shift covers a..c, got ${sel.keys.size}`);
  assert(!sel.keys.has(childX.key) && !sel.keys.has(otherPlugin.key), 'shift never leaves the parent');
}
{
  // Shift from a child of 'a' onto a root-level row is a boundary crossing.
  let sel = selectDiscoveryRow(emptyDiscoverySelection(), childX, visible);
  sel = selectDiscoveryRow(sel, c, visible, { shiftKey: true });
  assert(sel.keys.size === 1 && sel.keys.has(c.key), 'shift across parents collapses to a plain click');
}

// --- a node that left the snapshot leaves the selection ---
{
  let sel = selectDiscoveryRow(emptyDiscoverySelection(), a, visible);
  sel = selectDiscoveryRow(sel, b, visible, { ctrlKey: true });
  const afterRepublish = pruneDiscoverySelection(sel, [a, c, childX]);
  assert(afterRepublish.keys.size === 1 && afterRepublish.keys.has(a.key), 'the vanished node is dropped');
}

// --- an emptied selection returns to the empty value, which closes the menu ---
{
  const sel = selectDiscoveryRow(emptyDiscoverySelection(), a, visible);
  const pruned = pruneDiscoverySelection(sel, [b, c]);
  assert(pruned.keys.size === 0 && pruned.connectionId === '', 'everything gone → empty selection');
}

// --- selectedDiscoveryRows returns visible order, not click order ---
{
  let sel = selectDiscoveryRow(emptyDiscoverySelection(), c, visible);
  sel = selectDiscoveryRow(sel, a, visible, { ctrlKey: true });
  const picked = selectedDiscoveryRows(sel, visible).map((r) => r.nodeId);
  assert(picked.join(',') === 'a,c', `visible order, got ${picked.join(',')}`);
}

// --- a selection never leaks across connections ---
{
  const otherConn: DiscoveryRow = { ...row('p1', 'a'), connectionId: 'conn2' };
  const sel = selectDiscoveryRow(emptyDiscoverySelection(), a, visible);
  assert(
    selectedDiscoveryRows(sel, [otherConn]).length === 0,
    'a row with the same key under another connection is not the same row'
  );
  assert(pruneDiscoverySelection(sel, [otherConn]).keys.size === 0, 'prune is connection-scoped too');
}

// --- arrow keys walk every row; Shift+arrow stays inside the parent ---
{
  let sel = selectDiscoveryRow(emptyDiscoverySelection(), a, visible);
  sel = moveDiscoverySelection(sel, visible, 1, false);
  assert(sel.keys.size === 1 && sel.keys.has(b.key), 'Down moves to the next row');
  sel = moveDiscoverySelection(sel, visible, -1, false);
  assert(sel.keys.has(a.key), 'Up moves back');
  // A plain arrow may step into an expanded group's children.
  let intoChild = selectDiscoveryRow(emptyDiscoverySelection(), c, visible);
  intoChild = moveDiscoverySelection(intoChild, visible, 1, false);
  assert(intoChild.keys.has(childX.key), 'a plain arrow can step into a group');
  assert(intoChild.parentKey === childX.parentKey, 'and the selection follows to that parent');
}
{
  let sel = selectDiscoveryRow(emptyDiscoverySelection(), a, visible);
  sel = moveDiscoverySelection(sel, visible, 1, true);
  sel = moveDiscoverySelection(sel, visible, 1, true);
  assert(sel.keys.size === 3, `Shift+Down grows the selection, got ${sel.keys.size}`);
  // 'c' is the last sibling under this parent — Shift must stop, not wrap into
  // another parent's rows.
  const atEdge = moveDiscoverySelection(sel, visible, 1, true);
  assert(atEdge.keys.size === 3, 'Shift stops at the parent boundary');
  assert(!atEdge.keys.has(childX.key), 'and never reaches a child of a sibling');
}
{
  const empty = moveDiscoverySelection(emptyDiscoverySelection(), visible, 1, false);
  assert(empty.keys.size === 0, 'arrows do nothing when nothing is selected');
}

console.log('discoverySelection.test.ts: all passed');
