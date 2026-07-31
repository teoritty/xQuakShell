import type { DiscoveryNode, DiscoverySnapshot } from '../../api/discovery';
import { buildDiscoverySubtree, observedNodeIds } from './discoveryTree';
import { discoveryKey, type DiscoveryRow, type TreeNode } from './types';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

function node(partial: Partial<DiscoveryNode> & { id: string }): DiscoveryNode {
  return {
    parentId: '',
    kind: 'instance',
    label: partial.id,
    order: 0,
    actions: [],
    ...partial,
  };
}

function build(snapshot: DiscoverySnapshot, expanded: string[] = []): TreeNode[] {
  return buildDiscoverySubtree({
    connectionId: 'conn1',
    snapshot,
    expandedKeys: new Set(['', ...expanded]),
    baseDepth: 1,
    parentId: 'conn1',
  });
}

function rows(out: TreeNode[]): DiscoveryRow[] {
  return out.filter((n) => n.discovery).map((n) => n.discovery as DiscoveryRow);
}

// --- ordering: order, then label, then pluginId ---
{
  const snapshot: DiscoverySnapshot = {
    connectionId: 'conn1',
    plugins: [
      {
        pluginId: 'zeta',
        nodes: [
          node({ id: 'b', label: 'Same', order: 5 }),
          node({ id: 'a', label: 'Alpha', order: 5 }),
        ],
        branches: { '': { state: 'ready' } },
      },
      {
        pluginId: 'alpha',
        // Same order AND same label as zeta's 'b' — only pluginId can decide.
        nodes: [node({ id: 'b', label: 'Same', order: 5 }), node({ id: 'first', label: 'Zzz', order: 1 })],
        branches: { '': { state: 'ready' } },
      },
    ],
  };
  const out = rows(build(snapshot));
  const labels = out.map((r) => `${r.pluginId}/${r.label}`);
  assert(
    labels.join('|') === 'alpha/Zzz|zeta/Alpha|alpha/Same|zeta/Same',
    `order → label → pluginId, got ${labels.join('|')}`
  );
}

// --- two plugins, identical node id: two distinct rows, distinct TreeNode ids ---
{
  const snapshot: DiscoverySnapshot = {
    connectionId: 'conn1',
    plugins: [
      { pluginId: 'p1', nodes: [node({ id: 'containers', label: 'Containers' })], branches: { '': { state: 'ready' } } },
      { pluginId: 'p2', nodes: [node({ id: 'containers', label: 'Containers' })], branches: { '': { state: 'ready' } } },
    ],
  };
  const out = build(snapshot);
  const real = out.filter((n) => n.discovery);
  assert(real.length === 2, `two plugins with the same node id → two rows, got ${real.length}`);
  assert(real[0].id !== real[1].id, 'row ids must differ or the keyed each block collapses them');
  const keys = new Set(rows(out).map((r) => r.key));
  assert(keys.size === 2, 'discovery keys must include pluginId');
}

// --- truncated draws a service line saying so ---
{
  const snapshot: DiscoverySnapshot = {
    connectionId: 'conn1',
    plugins: [
      {
        pluginId: 'p1',
        nodes: [node({ id: 'a' }), node({ id: 'b' })],
        branches: { '': { state: 'ready', truncated: { shown: 2, total: 900 } } },
      },
    ],
  };
  const out = build(snapshot);
  const notice = out.find((n) => n.notice?.kind === 'truncated');
  assert(!!notice, 'truncated branch must draw a service row');
  assert(notice!.notice!.text === 'Showing 2 of 900', `got "${notice!.notice!.text}"`);
  assert(!notice!.discovery, 'a service row is not a selectable node');
}

// --- loading / error branches ---
{
  const out = build({
    connectionId: 'conn1',
    plugins: [{ pluginId: 'p1', nodes: [], branches: { '': { state: 'loading' } } }],
  });
  assert(out.length === 1 && out[0].notice?.kind === 'loading', 'loading branch → indicator row');
}
{
  const out = build({
    connectionId: 'conn1',
    plugins: [{ pluginId: 'p1', nodes: [], branches: { '': { state: 'error', error: 'socket closed' } } }],
  });
  assert(out[0].notice?.kind === 'error' && out[0].notice.text === 'socket closed', 'error text is shown');
}

// --- stale dims the WHOLE subtree and blocks actions in it ---
{
  const snapshot: DiscoverySnapshot = {
    connectionId: 'conn1',
    plugins: [
      {
        pluginId: 'p1',
        nodes: [
          node({ id: 'g', kind: 'group', label: 'G' }),
          node({ id: 'child', parentId: 'g', label: 'C' }),
        ],
        branches: { '': { state: 'stale' }, g: { state: 'ready' } },
      },
    ],
  };
  const out = rows(build(snapshot, [discoveryKey('p1', 'g')]));
  assert(out.length === 2, `expanded group shows its child, got ${out.length}`);
  assert(out.every((r) => r.stale), 'stale propagates down the subtree');
  assert(out.every((r) => r.actionsBlocked), 'stale blocks actions across the subtree');
}

// --- status: absent stays absent, present-but-neutral stays present ---
{
  const snapshot: DiscoverySnapshot = {
    connectionId: 'conn1',
    plugins: [
      {
        pluginId: 'p1',
        nodes: [
          node({ id: 'quiet', order: 0 }),
          node({ id: 'neutral', order: 1, status: { tone: 'neutral' } }),
        ],
        branches: { '': { state: 'ready' } },
      },
    ],
  };
  const out = rows(build(snapshot));
  assert(out[0].status === null, 'no reported status → null, and the view draws no dot');
  assert(out[1].status?.tone === 'neutral', 'a reported neutral status survives as a value');
}

// --- collapsed groups contribute no rows; expanded ones nest by depth ---
{
  const snapshot: DiscoverySnapshot = {
    connectionId: 'conn1',
    plugins: [
      {
        pluginId: 'p1',
        nodes: [node({ id: 'g', kind: 'group' }), node({ id: 'c', parentId: 'g' })],
        branches: { '': { state: 'ready' }, g: { state: 'ready' } },
      },
    ],
  };
  assert(rows(build(snapshot)).length === 1, 'collapsed group hides its children');
  const expanded = rows(build(snapshot, [discoveryKey('p1', 'g')]));
  assert(expanded.length === 2 && expanded[0].key !== expanded[1].key, 'expanded group reveals children');
  const nodes = build(snapshot, [discoveryKey('p1', 'g')]).filter((n) => n.discovery);
  assert(nodes[0].depth === 2 && nodes[1].depth === 3, 'children indent one level deeper');
}

// --- an empty / missing snapshot is an ordinary state, and never silence ---
{
  const none = build({ connectionId: 'conn1', plugins: [] });
  assert(none.length === 1 && none[0].notice?.kind === 'empty', 'no plugins → an explicit "nothing here" row');
  assert(rows(none).length === 0, 'and no selectable node');
  const pending = buildDiscoverySubtree({
    connectionId: 'conn1',
    snapshot: undefined,
    expandedKeys: new Set(),
    baseDepth: 0,
    parentId: 'conn1',
  });
  assert(pending.length === 1 && pending[0].notice?.kind === 'loading', 'no snapshot yet → the observe is in flight');
}

// --- observe is a level: the FULL set, always including the root ---
{
  const keys = new Set(['', discoveryKey('p1', 'g'), discoveryKey('p2', 'g')]);
  const ids = observedNodeIds(keys, true).sort();
  assert(ids.join(',') === ',g', `root plus deduped node ids, got "${ids.join(',')}"`);
  assert(observedNodeIds(keys, false).length === 0, 'a collapsed connection observes nothing at all');
}

console.log('discoveryTree.test.ts: all passed');
