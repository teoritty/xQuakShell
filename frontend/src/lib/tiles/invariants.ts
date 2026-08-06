// frontend/src/lib/tiles/invariants.ts
// Guarantees a TileLayout stays valid after any structural change:
//  - every tile's activeTabId points at one of its tabs (or '');
//  - dividers reset to a clean 0.5 whenever the tile count changes;
//  - activeTileId always points at an existing tile.

import type { TileGroup, TileLayout, Orientation } from './types';

export function removeTab(t: TileGroup, tabId: string): TileGroup {
  const tabs = t.tabs.filter((id) => id !== tabId);
  const activeTabId =
    t.activeTabId === tabId ? tabs[tabs.length - 1] ?? '' : t.activeTabId;
  return { ...t, tabs, activeTabId };
}

export function fixActiveTab(t: TileGroup): TileGroup {
  if (t.tabs.includes(t.activeTabId)) return t;
  return { ...t, activeTabId: t.tabs[t.tabs.length - 1] ?? '' };
}

/**
 * Rebuilds a layout around a new `tiles` array, re-establishing every
 * invariant. Resets dividers to 0.5 when the tile count changed (keeps grids
 * clean per spec). Optionally sets a new orientation.
 */
export function withTiles(
  layout: TileLayout,
  tiles: TileGroup[],
  orientation?: Orientation,
): TileLayout {
  const fixed = tiles.map(fixActiveTab);
  const countChanged = fixed.length !== layout.tiles.length;
  const activeExists = fixed.some((t) => t.id === layout.activeTileId);
  return {
    tiles: fixed,
    orientation: orientation ?? layout.orientation,
    dividers: countChanged ? { main: 0.5, cross: 0.5 } : layout.dividers,
    activeTileId: activeExists ? layout.activeTileId : fixed[0]?.id ?? '',
  };
}
