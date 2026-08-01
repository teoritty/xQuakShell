// Builds the flat row list for one connection's plugin-drawn subtree (ADR-014).
//
// Deliberately separate from buildTree.ts: that module is about folders and
// connections — host-owned config the user edits — and this one is about a
// remote, plugin-published, entirely read-only reflection of a machine's state.
// buildTree.ts calls in here at the connection node and otherwise knows nothing
// about discovery.
import type {
  DiscoveryBranch,
  DiscoveryNode,
  DiscoveryPluginTree,
  DiscoverySnapshot,
} from '../../api/discovery';
import {
  discoveryKey,
  discoveryKeyNodeId,
  discoveryNodeId,
  type DiscoveryNoticeKind,
  type DiscoveryRow,
  type TreeNode,
} from './types';

export interface DiscoverySubtreeInput {
  connectionId: string;
  snapshot: DiscoverySnapshot | undefined | null;
  /** discoveryKey(pluginId, nodeId) for every expanded group of this connection. */
  expandedKeys: Set<string>;
  /** Depth of the connection row; discovery rows start one level in. */
  baseDepth: number;
  /** TreeNode.id of the connection row, used as parentId for the top level. */
  parentId: string;
}

interface PluginIndex {
  pluginId: string;
  childrenOf: Map<string, DiscoveryNode[]>;
  branches: Record<string, DiscoveryBranch>;
}

/**
 * Nodes arrive pre-order, so a single pass is enough to bucket them by parent.
 * Insertion order inside a bucket is the publish order; the explicit sort below
 * is what actually decides the display order.
 */
function indexPluginTree(tree: DiscoveryPluginTree): PluginIndex {
  const childrenOf = new Map<string, DiscoveryNode[]>();
  for (const node of tree.nodes ?? []) {
    const parent = node.parentId ?? '';
    const bucket = childrenOf.get(parent);
    if (bucket) bucket.push(node);
    else childrenOf.set(parent, [node]);
  }
  return { pluginId: tree.pluginId, childrenOf, branches: tree.branches ?? {} };
}

/**
 * Order, then label, then pluginId. The third key is not a tiebreaker of last
 * resort — two plugins legitimately publish nodes with the same order and the
 * same label, and without it their relative position would depend on map
 * iteration order in Go and flip between snapshots.
 */
function compareRows(
  a: { node: DiscoveryNode; pluginId: string },
  b: { node: DiscoveryNode; pluginId: string }
): number {
  if (a.node.order !== b.node.order) return a.node.order - b.node.order;
  const byLabel = a.node.label.localeCompare(b.node.label);
  if (byLabel !== 0) return byLabel;
  return a.pluginId.localeCompare(b.pluginId);
}

function noticeRow(
  connectionId: string,
  pluginId: string,
  parentNodeId: string,
  kind: DiscoveryNoticeKind,
  text: string,
  depth: number,
  parentId: string
): TreeNode {
  return {
    type: 'discovery',
    id: `${discoveryNodeId(connectionId, pluginId, parentNodeId)}\u001f#${kind}`,
    name: text,
    depth,
    parentId,
    notice: { kind, text },
  };
}

function branchNotices(
  index: PluginIndex,
  parentNodeId: string,
  ownRowCount: number,
  connectionId: string,
  depth: number,
  parentId: string
): TreeNode[] {
  const branch = index.branches[parentNodeId];
  if (!branch) return [];
  const out: TreeNode[] = [];
  if (branch.state === 'loading') {
    out.push(noticeRow(connectionId, index.pluginId, parentNodeId, 'loading', 'Loading…', depth, parentId));
  } else if (branch.state === 'error') {
    out.push(
      noticeRow(
        connectionId,
        index.pluginId,
        parentNodeId,
        'error',
        branch.error || 'Failed to load',
        depth,
        parentId
      )
    );
  } else if (branch.state === 'ready' && ownRowCount === 0 && !branch.truncated) {
    out.push(noticeRow(connectionId, index.pluginId, parentNodeId, 'empty', 'Nothing here', depth, parentId));
  }
  if (branch.truncated) {
    const { shown, total } = branch.truncated;
    out.push(
      noticeRow(
        connectionId,
        index.pluginId,
        parentNodeId,
        'truncated',
        `Showing ${shown} of ${total}`,
        depth,
        parentId
      )
    );
  }
  return out;
}

/**
 * Flattens the whole visible subtree in display order. Rows are returned flat
 * rather than nested: discovery has no per-row children array to recurse into,
 * and the connection row owns the single "expanded" flag that reveals them.
 */
export function buildDiscoverySubtree(input: DiscoverySubtreeInput): TreeNode[] {
  const { connectionId, snapshot, expandedKeys, baseDepth, parentId } = input;
  // An expanded connection always shows SOMETHING. Silence would be
  // indistinguishable from a bug: no snapshot yet means the observe is in
  // flight, and a snapshot with no plugins means no installed plugin draws
  // anything under this protocol.
  if (!snapshot || !Array.isArray(snapshot.plugins)) {
    return [noticeRow(connectionId, '', '', 'loading', 'Loading…', baseDepth + 1, parentId)];
  }
  if (snapshot.plugins.length === 0) {
    return [
      noticeRow(connectionId, '', '', 'empty', 'No discovered resources', baseDepth + 1, parentId),
    ];
  }

  // Deliberately NOT pre-sorted by pluginId. Array#sort is stable, so ordering
  // the plugins first would decide ties on its own and make the third sort key
  // in compareRows unfalsifiable — two mechanisms doing one job, neither of them
  // testable in isolation. Rows are ordered by compareRows alone; the only place
  // that needs a settled plugin order is the service lines, which sort
  // themselves at the point of emission.
  const indexes = snapshot.plugins.map(indexPluginTree);
  const out: TreeNode[] = [];

  function buildLevel(
    parentNodeId: string,
    levelIndexes: PluginIndex[],
    depth: number,
    rowParentId: string,
    inheritedStale: boolean,
    inheritedBlocked: boolean
  ): void {
    const candidates: {
      node: DiscoveryNode;
      pluginId: string;
      index: PluginIndex;
      stale: boolean;
      blocked: boolean;
    }[] = [];
    const perPluginCounts = new Map<string, number>();

    for (const index of levelIndexes) {
      const branch = index.branches[parentNodeId];
      // `stale` is host state, not plugin state: the leading session handed over
      // or the plugin is restarting, so nobody can confirm this branch right now.
      const stale = inheritedStale || branch?.state === 'stale';
      const blocked = inheritedBlocked || stale || branch?.state === 'error';
      const nodes = index.childrenOf.get(parentNodeId) ?? [];
      perPluginCounts.set(index.pluginId, nodes.length);
      for (const node of nodes) {
        candidates.push({ node, pluginId: index.pluginId, index, stale, blocked });
      }
    }

    candidates.sort(compareRows);

    for (const candidate of candidates) {
      const { node, pluginId, index, stale, blocked } = candidate;
      const key = discoveryKey(pluginId, node.id);
      const isGroup = node.kind === 'group';
      const expanded = isGroup && expandedKeys.has(key);
      const row: DiscoveryRow = {
        connectionId,
        pluginId,
        nodeId: node.id,
        key,
        parentKey: discoveryKey(pluginId, parentNodeId),
        kind: isGroup ? 'group' : 'instance',
        label: node.label,
        iconId: node.iconId ?? '',
        // Preserve the "not reported" / "reported neutral" distinction: only an
        // absent status renders no dot at all.
        status: node.status ?? null,
        actions: node.actions ?? [],
        defaultActionId: node.defaultActionId ?? '',
        branchState: index.branches[node.id]?.state ?? 'ready',
        stale,
        actionsBlocked: blocked,
        expanded,
      };
      const treeNode: TreeNode = {
        type: 'discovery',
        id: discoveryNodeId(connectionId, pluginId, node.id),
        name: node.label,
        depth,
        parentId: rowParentId,
        discovery: row,
        expanded,
      };
      out.push(treeNode);
      if (expanded) {
        buildLevel(node.id, [index], depth + 1, treeNode.id, stale, blocked);
      }
    }

    // Service lines come after the level's rows, in a settled plugin order so
    // two plugins reporting "loading" do not swap places between snapshots.
    const noticeIndexes = [...levelIndexes].sort((a, b) => a.pluginId.localeCompare(b.pluginId));
    for (const index of noticeIndexes) {
      out.push(
        ...branchNotices(
          index,
          parentNodeId,
          perPluginCounts.get(index.pluginId) ?? 0,
          connectionId,
          depth,
          rowParentId
        )
      );
    }
  }

  buildLevel('', indexes, baseDepth + 1, parentId, false, false);
  return out;
}

/**
 * The tooltip of a discovery row: the label, plus the plugin that drew it.
 *
 * Two plugins may legitimately publish a group called "Storage" under the same
 * connection — node ids are plugin-scoped and labels are free text — and the
 * label alone leaves the two rows indistinguishable. Naming the source is the
 * cheapest way to tell them apart without inventing a disambiguation rule that
 * only kicks in on collisions and is therefore never exercised.
 *
 * The pluginId is the fallback: the plugin list arrives asynchronously, and an
 * id is uglier than a name while still answering the question the tooltip is
 * there to answer.
 */
export function discoveryRowTitle(
  row: { label: string; pluginId: string },
  pluginNames?: Map<string, string> | null
): string {
  const source = pluginNames?.get(row.pluginId) || row.pluginId;
  if (!source) return row.label;
  return `${row.label} — ${source}`;
}

/**
 * The full observe set for a connection: every expanded group's node id, plus
 * '' for the connection root. `observe` is a level — the backend gets the whole
 * set every time and the frontend never computes a delta.
 *
 * Node ids, not composite keys: the wire verb addresses plugins by their own
 * node ids, and two plugins observing the same id is exactly the intended
 * fan-out.
 */
export function observedNodeIds(expandedKeys: Set<string>, rootExpanded: boolean): string[] {
  if (!rootExpanded) return [];
  const ids = new Set<string>(['']);
  for (const key of expandedKeys) {
    const nodeId = discoveryKeyNodeId(key);
    if (nodeId !== null) ids.add(nodeId);
  }
  return [...ids];
}
