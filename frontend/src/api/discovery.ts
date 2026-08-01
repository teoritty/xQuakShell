// Atomic discovery-subtree RPC wrappers (ADR-014).
//
// Error handling goes through callBackend rather than a hand-rolled
// try/catch/showError, so api/discovery.ts needs no value import from
// stores/appState (architecture.test.ts rule 1 keeps that boundary closed, and
// its allowlist is a closed list, not a place to add a line).
//
// The per-method presence guard lives INSIDE each callBackend body: the
// discovery handlers are optional on the bridge, an older backend simply does
// not expose them, and "this build has no discovery" must degrade to an empty
// subtree rather than surface as an error the user is asked to read.
//
// EVERYTHING here is addressed by connectionId. The backend's sessionId is a
// transport detail resolved in the leading session and it must never appear in
// this file, in the store, or in any component.
import { callBackend, callBackendVoid } from '../backend/callBackend';

/** Tone vocabulary from ADR-014. Filled only by the plugin. */
export type DiscoveryTone = 'ok' | 'warn' | 'error' | 'busy' | 'neutral' | 'unknown';

/** Host-owned branch state. `stale` = the leading session handed over. */
export type DiscoveryBranchState = 'loading' | 'ready' | 'error' | 'stale';

export interface DiscoveryStatus {
  tone: DiscoveryTone;
  color?: string;
  tooltip?: string;
}

export interface DiscoveryAction {
  id: string;
  label: string;
  iconId?: string;
  danger?: boolean;
  /** Confirmation prompt the plugin wants shown before the action runs. */
  confirm?: string;
  /** Eligible for a multi-selection of siblings. */
  multi?: boolean;
}

export interface DiscoveryNode {
  id: string;
  parentId: string;
  kind: 'group' | 'instance';
  label: string;
  /**
   * ALREADY RESOLVED by the usecase: a node without its own icon inherits the
   * nearest ancestor's, and only the backend store holds that chain. The
   * frontend must not re-derive inheritance — it only looks the id up in the
   * plugin's `discoveryIcons` map.
   */
  iconId?: string;
  order: number;
  /**
   * Optional on purpose. `undefined` means the plugin reported no status at all
   * (render no dot); a present entry with tone `neutral` means the plugin said
   * "neutral" (render a grey dot). Collapsing the two loses information.
   */
  status?: DiscoveryStatus;
  actions: DiscoveryAction[];
  defaultActionId?: string;
}

export interface DiscoveryTruncated {
  shown: number;
  total: number;
}

export interface DiscoveryBranch {
  state: DiscoveryBranchState;
  error?: string;
  truncated?: DiscoveryTruncated;
}

export interface DiscoveryPluginTree {
  pluginId: string;
  /** Pre-order: every node follows its parent, so one pass builds the tree. */
  nodes: DiscoveryNode[];
  /** Keyed by node id, with '' standing for the connection root. */
  branches: Record<string, DiscoveryBranch>;
}

export interface DiscoverySnapshot {
  connectionId: string;
  plugins: DiscoveryPluginTree[];
}

/** ADR-014 limit: nodes in one invokeAction. */
export const MAX_ACTION_NODES = 200;

export function emptyDiscoverySnapshot(connectionId: string): DiscoverySnapshot {
  return { connectionId, plugins: [] };
}

export async function getDiscoveryTree(connectionId: string): Promise<DiscoverySnapshot> {
  return callBackend(
    'Load discovered resources',
    emptyDiscoverySnapshot(connectionId),
    async (app) => {
      if (!app.GetDiscoveryTree) return emptyDiscoverySnapshot(connectionId);
      const snapshot = (await app.GetDiscoveryTree(connectionId)) as unknown as DiscoverySnapshot;
      // "No tree" is an ordinary state, not an error: normalize so callers never
      // have to special-case a null before iterating.
      if (!snapshot || !Array.isArray(snapshot.plugins)) {
        return emptyDiscoverySnapshot(connectionId);
      }
      return snapshot;
    }
  );
}

/**
 * Publishes the FULL set of currently expanded node ids for a connection.
 * `observe` is a level, not an edge — the frontend never sends a delta, and
 * '' addresses the connection root.
 */
export async function setDiscoveryObserved(connectionId: string, nodeIds: string[]): Promise<void> {
  return callBackendVoid('Observe discovered resources', async (app) => {
    if (!app.SetDiscoveryObserved) return;
    await app.SetDiscoveryObserved(connectionId, nodeIds);
  });
}

/**
 * Invokes one plugin-defined action over a list of nodes. `nodeIds` is always a
 * list, even for a single node: a single action is a mass action over a list of
 * one. `pluginId` is mandatory — node ids are plugin-chosen and two plugins may
 * legitimately publish the same one under the same connection.
 */
export async function invokeDiscoveryAction(
  connectionId: string,
  pluginId: string,
  nodeIds: string[],
  actionId: string
): Promise<void> {
  return callBackendVoid('Run discovered resource action', async (app) => {
    if (!app.InvokeDiscoveryAction) return;
    await app.InvokeDiscoveryAction(connectionId, pluginId, nodeIds, actionId);
  });
}
