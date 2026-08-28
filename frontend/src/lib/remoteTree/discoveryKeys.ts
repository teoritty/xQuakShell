import { get } from 'svelte/store';
import { discoverySelection, setDiscoveryNodeExpanded } from '../../stores/discoveryState';
import { moveDiscoverySelection } from './discoverySelection';
import { discoveryNodeId, type DiscoveryRow } from './types';

/**
 * What the key handler needs from the tree, so the handler itself holds no state.
 *
 * `activate` is deliberately a callback rather than something resolved here: what Enter does on a
 * discovery row is the plugin's `defaultActionId`, and the core has no business deciding it.
 */
export interface DiscoveryKeyContext {
  rows: DiscoveryRow[];
  activate(row: DiscoveryRow): void;
}

/**
 * Keyboard navigation for a discovery row: Enter runs the default action, the arrows walk and
 * expand the subtree the way they walk folders.
 *
 * Returns true when the key was handled, so the caller can leave the ordinary tree keys alone.
 */
export function handleDiscoveryRowKey(
  event: KeyboardEvent,
  row: DiscoveryRow,
  ctx: DiscoveryKeyContext,
): boolean {
  if (event.key === 'Enter') {
    event.preventDefault();
    ctx.activate(row);
    return true;
  }
  if (event.key === 'ArrowRight' && row.kind === 'group' && !row.expanded) {
    event.preventDefault();
    void setDiscoveryNodeExpanded(row.connectionId, row.key, true);
    return true;
  }
  if (event.key === 'ArrowLeft' && row.kind === 'group' && row.expanded) {
    event.preventDefault();
    void setDiscoveryNodeExpanded(row.connectionId, row.key, false);
    return true;
  }
  if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') return false;

  // Shift extends, but only across siblings — that invariant lives in moveDiscoverySelection.
  event.preventDefault();
  const direction = event.key === 'ArrowDown' ? 1 : -1;
  discoverySelection.update((sel) => moveDiscoverySelection(sel, ctx.rows, direction, event.shiftKey));
  // Focus follows the selection so Enter and the left/right arrows keep acting on the row the user
  // can see is current. Resolved through the selection's own connectionId, so it can only land in
  // the subtree the selection lives in.
  const moved = get(discoverySelection);
  void focusDiscoveryRow(
    ctx.rows.find((r) => r.connectionId === moved.connectionId && r.key === moved.lastKey),
  );
  return true;
}

/**
 * Keeps DOM focus on the row the selection just moved to.
 *
 * Without this, the arrows would move the highlight while Enter and the left/right arrows kept
 * reading the previously focused row — the user would see one row selected and the keyboard would
 * act on another.
 */
export async function focusDiscoveryRow(row: DiscoveryRow | undefined): Promise<void> {
  if (!row || typeof document === 'undefined') return;
  // Addressed by TreeNode.id, which is scoped by (connectionId, pluginId, nodeId) — NOT by
  // discoveryKey, which omits the connection. Two connections showing the same plugin's
  // `containers` produce two rows with the same key, and querySelector would return whichever came
  // first in the document, so focus could land in a different connection's subtree.
  const id = discoveryNodeId(row.connectionId, row.pluginId, row.nodeId);
  const escaped =
    typeof CSS !== 'undefined' && CSS.escape ? CSS.escape(id) : id.replace(/["\\]/g, '\\$&');
  document
    .querySelector<HTMLElement>(`.remote-tree .tree-node[data-discovery-id="${escaped}"]`)
    ?.focus();
}
