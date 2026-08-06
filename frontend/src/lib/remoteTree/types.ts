import type { Connection, Folder } from '../../stores/appState';
import type {
  DiscoveryAction,
  DiscoveryBranchState,
  DiscoveryStatus,
} from '../../api/discovery';

export type ConnectionStatus = 'active' | 'connecting' | 'error' | 'disconnected';
export type DropZone = 'folder' | 'before' | 'after';

/**
 * Prefix on every discovery row's TreeNode.id.
 *
 * Discovery node ids are chosen by plugins and live in a different namespace
 * from connection/folder ids, which are host-generated. Prefixing makes a
 * collision structurally impossible rather than merely unlikely, so a plugin
 * cannot name a node after a real connection id and have it picked up by a
 * selection helper that filters by id. selection.ts additionally strips any id
 * carrying this prefix before it maps a selection onto real connections or
 * folders — belt and braces, because the failure mode there is deleting a
 * connection the user never selected.
 */
export const DISCOVERY_ID_PREFIX = 'discovery\u0000';

/**
 * Separator inside composite discovery keys. A C0 control character: labels and
 * ids are stripped of control characters upstream (ADR-014 security model), so
 * it cannot occur inside a plugin id or a node id and the composite key stays
 * unambiguous.
 */
const KEY_SEP = '\u001f';

/** Stable per-(plugin, node) key. Includes pluginId: two plugins may publish the same node id. */
export function discoveryKey(pluginId: string, nodeId: string): string {
  return `${pluginId}${KEY_SEP}${nodeId}`;
}

/** TreeNode.id for a discovery row — unique across connections, plugins and nodes. */
export function discoveryNodeId(connectionId: string, pluginId: string, nodeId: string): string {
  return `${DISCOVERY_ID_PREFIX}${connectionId}${KEY_SEP}${discoveryKey(pluginId, nodeId)}`;
}

/** The node id half of a discoveryKey, or null when the key is malformed. */
export function discoveryKeyNodeId(key: string): string | null {
  const sep = key.indexOf(KEY_SEP);
  if (sep < 0) return null;
  return key.slice(sep + 1);
}

export function isDiscoveryNodeId(id: string): boolean {
  return id.startsWith(DISCOVERY_ID_PREFIX);
}

/** A service line inside a subtree: host state, never a plugin-published node. */
export type DiscoveryNoticeKind = 'loading' | 'error' | 'truncated' | 'empty';

export interface DiscoveryRow {
  connectionId: string;
  pluginId: string;
  nodeId: string;
  /** discoveryKey(pluginId, nodeId). */
  key: string;
  /**
   * discoveryKey of the owning parent. Always plugin-scoped, including at the
   * connection root, so "children of one parent" also implies "one plugin" —
   * which is what makes a multi-selection addressable by a single pluginId.
   */
  parentKey: string;
  kind: 'group' | 'instance';
  label: string;
  /** Already-resolved icon id; look it up in the plugin's discoveryIcons map. */
  iconId: string;
  /**
   * null = the plugin reported no status (draw nothing). A present entry with
   * tone 'neutral' is a different thing and draws a grey dot.
   */
  status: DiscoveryStatus | null;
  actions: DiscoveryAction[];
  defaultActionId: string;
  /** Host state of THIS node's own children branch. */
  branchState: DiscoveryBranchState;
  /** This row's branch (or an ancestor's) is stale — dim the whole subtree. */
  stale: boolean;
  /** Actions are blocked here: the owning branch is stale or errored. */
  actionsBlocked: boolean;
  expanded: boolean;
}

export interface TreeNode {
  type: 'folder' | 'connection' | 'discovery';
  id: string;
  name: string;
  depth: number;
  parentId: string;
  folder?: Folder;
  connection?: Connection;
  children?: TreeNode[];
  expanded?: boolean;
  tags?: string[];
  /** Set only when type === 'discovery' and the row is a real plugin node. */
  discovery?: DiscoveryRow;
  /** Set only when type === 'discovery' and the row is a host service line. */
  notice?: { kind: DiscoveryNoticeKind; text: string };
}

export interface DragPayload {
  folderIds: string[];
  connectionIds: string[];
}

export interface DragVisualState {
  dragOverId: string | null;
  dragOverRoot: boolean;
  dragOverDropZone: DropZone | null;
  dragOverTargetId: string | null;
}

export const emptyDragVisualState = (): DragVisualState => ({
  dragOverId: null,
  dragOverRoot: false,
  dragOverDropZone: null,
  dragOverTargetId: null,
});
