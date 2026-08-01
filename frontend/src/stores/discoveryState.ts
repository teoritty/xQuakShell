// Client state for plugin-drawn subtrees (ADR-014).
//
// Everything is keyed by connectionId. The backend's sessionId is resolved in
// the leading session and never crosses the Wails seam, so there is deliberately
// no session-shaped key anywhere in this file.
//
// Nothing here is persisted. Discovery reflects remote reality, not local
// config: a tree restored from disk would describe containers that stopped
// hours ago, and a stale cache is worse than an empty subtree.
import { get, writable } from 'svelte/store';

import {
  getDiscoveryTree,
  setDiscoveryObserved,
  type DiscoverySnapshot,
} from '../api/discovery';
import { listPlugins } from '../api/plugins';
import { observedNodeIds } from '../lib/remoteTree/discoveryTree';
import {
  emptyDiscoverySelection,
  pruneDiscoverySelection,
  type DiscoverySelection,
} from '../lib/remoteTree/discoverySelection';
import { discoveryKey, type DiscoveryRow } from '../lib/remoteTree/types';

/** connectionId -> last snapshot fetched for it. */
export const discoverySnapshots = writable<Map<string, DiscoverySnapshot>>(new Map());

/**
 * connectionId -> expanded discoveryKey set. The connection root is the empty
 * key '': a connection whose set contains it is showing its subtree, and one
 * whose set does not is observing nothing at all.
 */
export const discoveryExpanded = writable<Map<string, Set<string>>>(new Map());

/**
 * `${pluginId}${iconId}` -> data URI. Scoped by plugin because iconIds are
 * plugin-chosen and two plugins may both ship an icon called "volumes".
 */
export const discoveryIcons = writable<Map<string, string>>(new Map());

/**
 * Discovery's own selection. It is a separate store from selectedConnectionIds /
 * selectedFolderId and the two never merge — see discoverySelection.ts for why
 * that separation is load-bearing rather than tidy.
 */
export const discoverySelection = writable<DiscoverySelection>(emptyDiscoverySelection());

/** Key for the discoveryIcons map. */
export function discoveryIconKey(pluginId: string, iconId: string): string {
  return discoveryKey(pluginId, iconId);
}

export function isDiscoveryRootExpanded(expanded: Map<string, Set<string>>, connectionId: string): boolean {
  return expanded.get(connectionId)?.has('') ?? false;
}

export function discoveryExpandedKeys(
  expanded: Map<string, Set<string>>,
  connectionId: string
): Set<string> {
  return expanded.get(connectionId) ?? new Set();
}

/**
 * pluginId -> display name, for the row tooltip. Two plugins may draw a group
 * with the same label under one connection, and the tooltip is what tells them
 * apart (see discoveryRowTitle).
 */
export const discoveryPluginNames = writable<Map<string, string>>(new Map());

/**
 * Fills both maps from one ListPlugins call. The names ride along rather than
 * getting a refresher of their own: they come from the same response, change at
 * the same moments (install, update, removal), and a second round trip for a
 * tooltip would be a request nobody is waiting for.
 */
export async function refreshDiscoveryIcons(): Promise<void> {
  const plugins = await listPlugins();
  const next = new Map<string, string>();
  const names = new Map<string, string>();
  for (const plugin of plugins) {
    if (plugin.name) names.set(plugin.id, plugin.name);
    for (const [iconId, dataUri] of Object.entries(plugin.discoveryIcons ?? {})) {
      next.set(discoveryIconKey(plugin.id, iconId), dataUri);
    }
  }
  discoveryIcons.set(next);
  discoveryPluginNames.set(names);
}

export async function refreshDiscoveryTree(connectionId: string): Promise<void> {
  if (!connectionId) return;
  const snapshot = await getDiscoveryTree(connectionId);
  discoverySnapshots.update((map) => {
    const next = new Map(map);
    next.set(connectionId, snapshot);
    return next;
  });
}

/**
 * Drops every trace of one connection's subtree: snapshot, expansion and
 * selection.
 *
 * Nothing here is repaired lazily. `discoveryExpanded` is what makes buildTree
 * emit rows at all, and buildTree knows nothing about sessions — so a subtree
 * whose expansion survives its session keeps rendering with no way left to
 * collapse it, because the only control that collapses it is the connection
 * row's arrow, and that arrow is gone.
 */
export function forgetDiscoveryTree(connectionId: string): void {
  discoverySnapshots.update((map) => {
    if (!map.has(connectionId)) return map;
    const next = new Map(map);
    next.delete(connectionId);
    return next;
  });
  discoveryExpanded.update((map) => {
    if (!map.has(connectionId)) return map;
    const next = new Map(map);
    next.delete(connectionId);
    return next;
  });
  discoverySelection.update((sel) =>
    sel.connectionId === connectionId ? emptyDiscoverySelection() : sel
  );
}

/**
 * Forgets the subtree of every connection that can no longer have one.
 *
 * Discovery enumerates through a leading session; when the last `ready` session
 * of a connection closes the backend deletes its tree outright (ADR-014: nothing
 * is cached or persisted). The frontend has to follow, and it cannot wait to be
 * told — DiscoveryTreeChanged is about a tree that still exists.
 *
 * Called reactively from the tree with the same set that decides whether the
 * connection row draws its expander, so the rows and the control that hides them
 * can never disagree.
 */
export function forgetUnavailableDiscovery(availableConnectionIds: Set<string>): void {
  for (const connectionId of [...get(discoveryExpanded).keys()]) {
    if (!availableConnectionIds.has(connectionId)) forgetDiscoveryTree(connectionId);
  }
  // A connection may hold a snapshot without an expansion for a moment after a
  // refresh raced a collapse; clear those too rather than leak them.
  for (const connectionId of [...get(discoverySnapshots).keys()]) {
    if (!availableConnectionIds.has(connectionId)) forgetDiscoveryTree(connectionId);
  }
}

/**
 * Republishes the FULL observe set for one connection. Never a delta: `observe`
 * is a level, so a lost or reordered message repairs itself on the next call,
 * and a plugin that restarts is told everything again without a resync verb.
 */
async function publishObserved(connectionId: string): Promise<void> {
  const expanded = get(discoveryExpanded);
  const keys = discoveryExpandedKeys(expanded, connectionId);
  await setDiscoveryObserved(connectionId, observedNodeIds(keys, keys.has('')));
}

function mutateExpanded(connectionId: string, mutate: (set: Set<string>) => void): void {
  discoveryExpanded.update((map) => {
    const next = new Map(map);
    const set = new Set(next.get(connectionId) ?? []);
    mutate(set);
    if (set.size === 0) next.delete(connectionId);
    else next.set(connectionId, set);
    return next;
  });
}

/** Expands/collapses a connection's whole subtree (the '' root observation). */
export async function toggleDiscoveryRoot(connectionId: string): Promise<void> {
  const wasExpanded = isDiscoveryRootExpanded(get(discoveryExpanded), connectionId);
  mutateExpanded(connectionId, (set) => {
    if (wasExpanded) set.clear();
    else set.add('');
  });
  if (wasExpanded) {
    // Collapsing drops the snapshot as well as the observation: the plugin stops
    // polling a branch nobody is looking at, which is the load relief `observe`
    // exists for, and keeping the rows around would only let them go stale.
    discoverySnapshots.update((map) => {
      if (!map.has(connectionId)) return map;
      const next = new Map(map);
      next.delete(connectionId);
      return next;
    });
    discoverySelection.update((sel) =>
      sel.connectionId === connectionId ? emptyDiscoverySelection() : sel
    );
  }
  await publishObserved(connectionId);
  if (!wasExpanded) await refreshDiscoveryTree(connectionId);
}

export async function setDiscoveryNodeExpanded(
  connectionId: string,
  key: string,
  expanded: boolean
): Promise<void> {
  mutateExpanded(connectionId, (set) => {
    if (expanded) set.add(key);
    else set.delete(key);
  });
  await publishObserved(connectionId);
  if (expanded) await refreshDiscoveryTree(connectionId);
}

export async function toggleDiscoveryNode(connectionId: string, key: string): Promise<void> {
  const isExpanded = discoveryExpandedKeys(get(discoveryExpanded), connectionId).has(key);
  await setDiscoveryNodeExpanded(connectionId, key, !isExpanded);
}

/**
 * Called after the visible rows are rebuilt. A node that vanished from a
 * republished snapshot must leave the selection, otherwise an action could still
 * be aimed at a node id the plugin has forgotten.
 */
export function reconcileDiscoverySelection(visibleRows: DiscoveryRow[]): void {
  discoverySelection.update((sel) => pruneDiscoverySelection(sel, visibleRows));
}

export function clearDiscoverySelection(): void {
  discoverySelection.set(emptyDiscoverySelection());
}

/**
 * Handles the backend's DiscoveryTreeChanged event. The payload names the node
 * that changed, but the read side is a whole-connection snapshot, so the node id
 * is only a hint — refetching the connection is both correct and simpler than
 * splicing one branch into a tree the backend already recomposed.
 */
export function onDiscoveryTreeChanged(connectionId: string): void {
  if (!connectionId) return;
  if (!isDiscoveryRootExpanded(get(discoveryExpanded), connectionId)) return;
  void refreshDiscoveryTree(connectionId);
}
