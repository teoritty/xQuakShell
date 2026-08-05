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
