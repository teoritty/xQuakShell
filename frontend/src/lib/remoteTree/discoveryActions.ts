// Computes the action menu for a discovery selection (ADR-014).
//
// A pure function over rows, with no DOM and no store access, because every
// interesting rule here is a rule about a set — an intersection, a limit, a
// branch state — and those are worth testing without a component around them.
//
// Actions are fully opaque to the core: it draws the label, relays the click and
// knows nothing about what happens next.
import { MAX_ACTION_NODES, type DiscoveryAction } from '../../api/discovery';
import type { DiscoveryRow } from './types';

export interface DiscoveryMenuItem {
  id: string;
  label: string;
  iconId: string;
  danger: boolean;
  /** Non-empty means: show this prompt and get a yes before invoking. */
  confirm: string;
  disabled: boolean;
}

export interface DiscoveryMenu {
  items: DiscoveryMenuItem[];
  /**
   * Why the menu is empty or disabled. Never left blank when `items` is empty:
   * an action menu that simply is not there reads as a bug, whereas "no action
   * applies to all 4 selected items" is an answer.
   */
  notice: string;
  /** Node ids to pass to InvokeDiscoveryAction, in visible order. */
  nodeIds: string[];
  pluginId: string;
  connectionId: string;
}

function emptyMenu(notice: string): DiscoveryMenu {
  return { items: [], notice, nodeIds: [], pluginId: '', connectionId: '' };
}

function toItem(action: DiscoveryAction, disabled: boolean): DiscoveryMenuItem {
  return {
    id: action.id,
    label: action.label,
    iconId: action.iconId ?? '',
    danger: !!action.danger,
    confirm: action.confirm ?? '',
    disabled,
  };
}

export function computeDiscoveryMenu(rows: DiscoveryRow[]): DiscoveryMenu {
  if (rows.length === 0) return emptyMenu('Nothing selected');

  const first = rows[0];
  const nodeIds = rows.map((r) => r.nodeId);
  const base = { nodeIds, pluginId: first.pluginId, connectionId: first.connectionId };

  // A stale or errored branch means nobody can confirm the tree on screen right
  // now — the leading session handed over, or the plugin is restarting. The rows
  // stay visible (an empty tree would be worse) but every action is refused,
  // because the node ids may already name resources that no longer exist.
  const blocked = rows.some((r) => r.actionsBlocked);

  let candidates: DiscoveryAction[];
  if (rows.length === 1) {
    candidates = first.actions;
  } else {
    // Intersection by actionId across every selected row, restricted to actions
    // the plugin marked multi. An action without `multi` is not merely
    // unhelpful in bulk — the plugin has said it is not safe there.
    const multiOf = (row: DiscoveryRow) =>
      new Map(row.actions.filter((a) => a.multi).map((a) => [a.id, a]));
    const firstMulti = multiOf(first);
    const others = rows.slice(1).map(multiOf);
    candidates = [...firstMulti.values()].filter((a) => others.every((m) => m.has(a.id)));
  }

  if (candidates.length === 0) {
    const notice =
      rows.length === 1
        ? 'No actions for this item'
        : `No action applies to all ${rows.length} selected items`;
    return { ...emptyMenu(notice), ...base };
  }

  // Checked BEFORE the size limit: both disable the menu, so only the notice
  // differs, and a stale branch is the more fundamental reason. Told about the
  // limit instead, the user would trim the selection and still be refused,
  // having been given the wrong explanation twice.
  if (blocked) {
    return {
      ...base,
      items: candidates.map((a) => toItem(a, true)),
      notice: 'This branch is out of date — actions are unavailable until it refreshes',
    };
  }

  if (rows.length > MAX_ACTION_NODES) {
    return {
      ...base,
      items: candidates.map((a) => toItem(a, true)),
      notice: `Actions run on at most ${MAX_ACTION_NODES} items at a time (${rows.length} selected)`,
    };
  }

  return { ...base, items: candidates.map((a) => toItem(a, false)), notice: '' };
}

/**
 * The action a double-click or Enter runs. Only for a single row: a default
 * action is a per-node convenience, and firing one across a multi-selection
 * without the user naming it is not a convenience.
 */
export function defaultDiscoveryAction(rows: DiscoveryRow[]): DiscoveryMenuItem | null {
  if (rows.length !== 1) return null;
  const row = rows[0];
  if (!row.defaultActionId || row.actionsBlocked) return null;
  const action = row.actions.find((a) => a.id === row.defaultActionId);
  return action ? toItem(action, false) : null;
}
