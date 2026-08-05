import type { DiscoveryAction } from '../../api/discovery';
import { computeDiscoveryMenu, defaultDiscoveryAction, deleteMenuItem } from './discoveryActions';
import { discoveryKey, type DiscoveryRow } from './types';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

function row(nodeId: string, actions: DiscoveryAction[], extra: Partial<DiscoveryRow> = {}): DiscoveryRow {
  return {
    connectionId: 'conn1',
    pluginId: 'p1',
    nodeId,
    key: discoveryKey('p1', nodeId),
    parentKey: discoveryKey('p1', ''),
    kind: 'instance',
    label: nodeId,
    iconId: '',
    status: null,
    actions,
    defaultActionId: '',
    branchState: 'ready',
    stale: false,
    actionsBlocked: false,
    expanded: false,
    ...extra,
  };
}

const start: DiscoveryAction = { id: 'start', label: 'Start', multi: true };
const stop: DiscoveryAction = { id: 'stop', label: 'Stop', multi: true, danger: true, confirm: 'Stop it?' };
const logs: DiscoveryAction = { id: 'logs', label: 'Logs' }; // NOT multi
const inspect: DiscoveryAction = { id: 'inspect', label: 'Inspect' };
const remove: DiscoveryAction = { id: 'remove', label: 'Remove…', multi: true, danger: true, delete: true };

// --- nothing selected ---
{
  const menu = computeDiscoveryMenu([]);
  assert(menu.items.length === 0 && menu.notice === 'Nothing selected', 'empty selection is explained');
}

// --- one node offers exactly its own actions, multi or not ---
{
  const menu = computeDiscoveryMenu([row('a', [start, logs])]);
  assert(menu.items.map((i) => i.id).join(',') === 'start,logs', 'single node keeps non-multi actions');
  assert(menu.notice === '', 'a populated menu carries no notice');
  assert(menu.nodeIds.join(',') === 'a' && menu.pluginId === 'p1', 'menu carries its addressee');
  const stopItem = computeDiscoveryMenu([row('a', [stop])]).items[0];
  assert(stopItem.danger && stopItem.confirm === 'Stop it?', 'danger and confirm survive into the item');
}

// --- several nodes: intersection restricted to multi:true ---
{
  const menu = computeDiscoveryMenu([row('a', [start, stop, logs]), row('b', [start, logs, inspect])]);
  assert(
    menu.items.map((i) => i.id).join(',') === 'start',
    `only multi actions present on every row survive, got ${menu.items.map((i) => i.id).join(',')}`
  );
}
{
  // 'logs' is on both rows but is not multi — it must not appear.
  const menu = computeDiscoveryMenu([row('a', [logs]), row('b', [logs])]);
  assert(menu.items.length === 0, 'a shared but non-multi action is not offered in bulk');
  assert(
    menu.notice === 'No action applies to all 2 selected items',
    `empty intersection is explained, got "${menu.notice}"`
  );
}

// --- more than 200 nodes: items shown but blocked, with the reason ---
{
  const many = Array.from({ length: 201 }, (_, i) => row(`n${i}`, [start]));
  const menu = computeDiscoveryMenu(many);
  assert(menu.items.length === 1 && menu.items[0].disabled, '>200 disables the items rather than hiding them');
  assert(menu.notice.includes('200') && menu.notice.includes('201'), `notice names both limits: "${menu.notice}"`);
  // Exactly 200 is still allowed — the limit is "no more than", not "fewer than".
  assert(!computeDiscoveryMenu(many.slice(0, 200)).items[0].disabled, '200 nodes is within the limit');
}

// --- a stale or errored branch blocks everything in it ---
{
  const menu = computeDiscoveryMenu([row('a', [start], { actionsBlocked: true })]);
  assert(menu.items.length === 1 && menu.items[0].disabled, 'stale branch disables the action');
  assert(menu.notice.length > 0, 'and says why');
}
{
  // One blocked row poisons a mixed selection: the set is acted on as a whole.
  const menu = computeDiscoveryMenu([row('a', [start]), row('b', [start], { actionsBlocked: true })]);
  assert(menu.items.every((i) => i.disabled), 'any blocked row blocks the whole selection');
}
{
  // Both reasons at once: the notice must name the stale branch, not the size
  // limit. Trimming the selection would not help, so saying "at most 200" would
  // send the user down a road that ends in the same refusal.
  const many = Array.from({ length: 201 }, (_, i) => row(`n${i}`, [start], { actionsBlocked: true }));
  const menu = computeDiscoveryMenu(many);
  assert(menu.items.every((i) => i.disabled), 'still disabled');
  assert(
    menu.notice.includes('out of date') && !menu.notice.includes('200'),
    `a stale branch outranks the size limit in the explanation, got "${menu.notice}"`
  );
}

// --- defaultActionId is a single-row affordance only ---
{
  const single = row('a', [start, logs], { defaultActionId: 'logs' });
  assert(defaultDiscoveryAction([single])?.id === 'logs', 'double-click/Enter resolves defaultActionId');
  assert(defaultDiscoveryAction([single, row('b', [start])]) === null, 'no default action across a selection');
  assert(defaultDiscoveryAction([row('a', [start])]) === null, 'no defaultActionId → nothing to run');
  assert(
    defaultDiscoveryAction([row('a', [start], { defaultActionId: 'gone' })]) === null,
    'a defaultActionId naming an absent action resolves to nothing'
  );
  assert(
    defaultDiscoveryAction([{ ...single, actionsBlocked: true }]) === null,
    'a blocked branch has no default action either'
  );
}

// --- the Delete key resolves through the menu, never around it ---
{
  assert(
    deleteMenuItem(computeDiscoveryMenu([row('a', [start, remove])]))?.id === 'remove',
    'the marked action is what Delete runs'
  );
  assert(
    deleteMenuItem(computeDiscoveryMenu([row('a', [start, logs])])) === null,
    'no marked action means the key does nothing'
  );
  assert(deleteMenuItem(computeDiscoveryMenu([])) === null, 'nothing selected, nothing to delete');
  // Every menu rule the key inherits, checked at the boundary that enforces it.
  assert(
    deleteMenuItem(computeDiscoveryMenu([row('a', [remove]), row('b', [start])])) === null,
    'an action missing from one selected row is out of reach for the key too'
  );
  const notMulti: DiscoveryAction = { ...remove, multi: false };
  assert(
    deleteMenuItem(computeDiscoveryMenu([row('a', [notMulti]), row('b', [notMulti])])) === null,
    'a delete the plugin did not mark multi is not reachable across a selection'
  );
  assert(
    deleteMenuItem(computeDiscoveryMenu([row('a', [remove], { actionsBlocked: true })])) === null,
    'a stale branch refuses the key exactly as it refuses the menu'
  );
  const second: DiscoveryAction = { id: 'purge', label: 'Purge', multi: true, delete: true };
  assert(
    deleteMenuItem(computeDiscoveryMenu([row('a', [remove, second])])) === null,
    'two marked actions is an unanswered question, not a coin flip'
  );
}

console.log('discoveryActions.test.ts: all passed');
