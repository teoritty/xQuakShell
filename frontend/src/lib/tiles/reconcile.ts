// frontend/src/lib/tiles/reconcile.ts
// Keeps a TileLayout in sync with the authoritative list of open tabs. A tab is
// either an SSH session (stores/appState) or a plugin-owned surface
// (stores/surfaceState, ADR-015); both stores stay the single source of truth
// for lifecycle, and this is the ONLY place a tile learns that one appeared or
// disappeared.
//
// The ids are opaque here on purpose. Nothing below asks what a tab IS — that
// question is answered exactly twice, in TileGroup (which renders the body) and
// TileTabBar (which renders the label). Teaching the placement machinery the
// difference would put a second lifecycle rule next to this one.

import type { TileLayout, TileGroup } from './types';
import { emptyTile } from './types';
import { withTiles, fixActiveTab } from './invariants';

export function reconcile(
  layout: TileLayout,
  tabIds: string[],
  activeTabId: string,
): TileLayout {
  const present = new Set(tabIds);

  // 1. Strip tabs whose session or surface is gone.
  const stripped = layout.tiles
    .map((t) => ({ ...t, tabs: t.tabs.filter((id) => present.has(id)) }))
    .map(fixActiveTab);

  // 2. Keep non-empty tiles; if all empty, keep exactly one empty tile.
  let tiles: TileGroup[] = stripped.filter((t) => t.tabs.length > 0);
  if (tiles.length === 0) {
    const keep = stripped[0] ?? emptyTile();
    tiles = [{ ...keep, tabs: [], activeTabId: '' }];
  }

  // 3. Resolve the active tile (may have collapsed).
  let activeTileId = tiles.some((t) => t.id === layout.activeTileId)
    ? layout.activeTileId
    : tiles[0].id;

  // 4. Place newly-appeared tabs onto the active tile, in arrival order.
  const assigned = new Set(tiles.flatMap((t) => t.tabs));
  const fresh = tabIds.filter((id) => !assigned.has(id));
  if (fresh.length > 0) {
    tiles = tiles.map((t) =>
      t.id === activeTileId
        ? { ...t, tabs: [...t.tabs, ...fresh], activeTabId: fresh[fresh.length - 1] }
        : t,
    );
  }

  // 5. Focus follows activeTabId: its tile becomes active, that tab selected.
  if (activeTabId && present.has(activeTabId)) {
    const host = tiles.find((t) => t.tabs.includes(activeTabId));
    if (host) {
      activeTileId = host.id;
      tiles = tiles.map((t) =>
        t.id === host.id ? { ...t, activeTabId } : t,
      );
    }
  }

  const next = withTiles({ ...layout, activeTileId }, tiles);
  return { ...next, activeTileId };
}
