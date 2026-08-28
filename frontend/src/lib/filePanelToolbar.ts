import type { ComponentType } from 'svelte';
import {
  ArrowUpDown,
  Calendar,
  Eye,
  RefreshCw,
  Shield,
  User,
} from 'lucide-svelte';

export type SortKey = 'name' | 'size' | 'modTime' | 'owner';
export type SortDir = 'asc' | 'desc';

export interface ToolbarItem {
  id: string;
  label: string;
  icon: ComponentType;
  onClick: () => void;
  active?: boolean;
  disabled?: boolean;
  buttonClass?: string;
  /** Compact toolbar only (e.g. "N ↓"). */
  inlineSuffix?: string;
  /** Overflow menu only (e.g. " ↓" for active sort). */
  menuSuffix?: string;
  /** Show checkmark on the right in overflow menu when active. */
  showCheck?: boolean;
}

export interface FilePanelToolbarContext {
  showPermissions: boolean;
  showOwner: boolean;
  showDate: boolean;
  showHidden: boolean;
  sortEnabled: boolean;
  sortKey: SortKey | null;
  sortDir: SortDir;
  togglePermissions: () => void;
  toggleOwner: () => void;
  toggleDate: () => void;
  toggleHidden: () => void;
  toggleSort: (key: SortKey) => void;
  refresh: () => void;
  refreshDisabled?: boolean;
}

export function sortArrow(dir: SortDir): string {
  // Down = ascending (A→Z, small→large, old→new); up = descending.
  return dir === 'asc' ? ' ↓' : ' ↑';
}

export function cycleSortState(
  current: { sortEnabled: boolean; sortKey: SortKey | null; sortDir: SortDir },
  nextKey: SortKey
): { sortEnabled: boolean; sortKey: SortKey | null; sortDir: SortDir } {
  if (!current.sortEnabled || current.sortKey !== nextKey) {
    return { sortEnabled: true, sortKey: nextKey, sortDir: 'asc' };
  }
  if (current.sortDir === 'asc') {
    return { sortEnabled: true, sortKey: nextKey, sortDir: 'desc' };
  }
  return { sortEnabled: false, sortKey: null, sortDir: 'asc' };
}

/** Resolves a message key. The panels pass `$t`; tests pass whatever they need. */
export type ToolbarLabels = (key: string) => string;

/**
 * Builds the file panel toolbar.
 *
 * The one-letter badge on a sort button is translated along with its caption rather than derived
 * from it: it is a mnemonic for the word the user actually reads, so "N" for name is only a
 * mnemonic in a language where that word starts with an N.
 */
export function buildFilePanelToolbarItems(
  ctx: FilePanelToolbarContext,
  label: ToolbarLabels,
): ToolbarItem[] {
  const sortItem = (key: SortKey): ToolbarItem => {
    const active = ctx.sortEnabled && ctx.sortKey === key;
    const arrow = active ? sortArrow(ctx.sortDir) : '';
    const letter = label(`files.sort.${key}.letter`);
    return {
      id: `sort-${key}`,
      label: label(`files.sort.${key}`),
      icon: ArrowUpDown,
      buttonClass: 'sort-toggle',
      active,
      inlineSuffix: `${letter}${arrow}`,
      menuSuffix: arrow,
      onClick: () => ctx.toggleSort(key),
    };
  };

  return [
    {
      id: 'permissions',
      label: label('files.column.permissions'),
      icon: Shield,
      buttonClass: 'column-toggle',
      active: ctx.showPermissions,
      showCheck: true,
      onClick: ctx.togglePermissions,
    },
    {
      id: 'owner',
      label: label('files.column.owner'),
      icon: User,
      buttonClass: 'column-toggle',
      active: ctx.showOwner,
      showCheck: true,
      onClick: ctx.toggleOwner,
    },
    {
      id: 'date',
      label: label('files.column.date'),
      icon: Calendar,
      buttonClass: 'column-toggle',
      active: ctx.showDate,
      showCheck: true,
      onClick: ctx.toggleDate,
    },
    {
      id: 'hidden',
      label: label('files.column.hidden'),
      icon: Eye,
      buttonClass: 'column-toggle',
      active: ctx.showHidden,
      showCheck: true,
      onClick: ctx.toggleHidden,
    },
    sortItem('name'),
    sortItem('size'),
    sortItem('modTime'),
    sortItem('owner'),
    {
      id: 'refresh',
      label: label('common.refresh'),
      icon: RefreshCw,
      disabled: ctx.refreshDisabled,
      onClick: ctx.refresh,
    },
  ];
}
