// The discovery node whose property panel is open (ADR-015 §3).
//
// It mirrors detailsConnectionId: the sidebar shows the details of whatever the tree selection
// points at, and a node and a connection are two kinds of that one thing. They are separate stores
// because opening one must close the other — the panel has one slot.
import { writable, get } from 'svelte/store';
import { detailsConnectionId } from './appState';
import type { DialogSection } from './dialogState';

export interface NodeDetailsTarget {
  connectionId: string;
  pluginId: string;
  nodeId: string;
  label: string;
}

export interface NodeDetails {
  sections: DialogSection[];
  values: Record<string, string>;
  editable: boolean;
}

export const nodeDetailsTarget = writable<NodeDetailsTarget | null>(null);

/**
 * Bumped every time the owning plugin says its panel is stale (ADR-015 §3).
 *
 * A separate counter rather than a re-set of the target, because the target's identity is exactly
 * what does NOT change on a refresh: same connection, same plugin, same node. Re-setting it leaves
 * every derived value equal, so nothing downstream re-runs and the push is silently lost.
 */
export const nodeDetailsRevision = writable(0);

/** Opens the node panel, closing the connection editor: the sidebar has one details slot. */
export function openNodeDetails(target: NodeDetailsTarget): void {
  detailsConnectionId.set('');
  nodeDetailsTarget.set(target);
}

export function closeNodeDetails(): void {
  nodeDetailsTarget.set(null);
}

/**
 * Closes the node panel when a connection is selected instead.
 *
 * Called from the connection path rather than subscribed to it, so the two stores do not form a
 * cycle that fires on every keystroke in either panel.
 */
export function nodeDetailsYieldToConnection(): void {
  if (get(nodeDetailsTarget) !== null) nodeDetailsTarget.set(null);
}

/** True when a pushed refresh names the node currently on screen. */
export function isCurrentNode(connectionId: string, pluginId: string, nodeId: string): boolean {
  const target = get(nodeDetailsTarget);
  return (
    !!target &&
    target.connectionId === connectionId &&
    target.pluginId === pluginId &&
    target.nodeId === nodeId
  );
}

/**
 * Asks the open panel to re-read, if the push names the node it is showing.
 *
 * Returns whether it did, which is what makes the behaviour testable without a component: a plugin
 * may publish details for any node in its subtree, and only the one on screen is worth a round
 * trip back to it.
 */
export function requestNodeDetailsReload(
  connectionId: string,
  pluginId: string,
  nodeId: string
): boolean {
  if (!isCurrentNode(connectionId, pluginId, nodeId)) return false;
  nodeDetailsRevision.update((n) => n + 1);
  return true;
}
