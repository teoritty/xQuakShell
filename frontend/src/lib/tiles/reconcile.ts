// frontend/src/lib/tiles/reconcile.ts
// Keeps a TileLayout in sync with the authoritative session list. `sessions`
// (in stores/appState) stays the single source of truth for lifecycle; this is
// the ONLY place a tile learns that a session appeared or disappeared.

import type { TileLayout, TileGroup } from './types';
import { emptyTile } from './types';
import { withTiles, fixActiveTab } from './invariants';

export function reconcile(
  layout: TileLayout,
  sessionIds: string[],
  activeSessionId: string,
): TileLayout {
  const present = new Set(sessionIds);

  // 1. Strip tabs whose session is gone.
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

  // 4. Place newly-appeared sessions onto the active tile, in arrival order.
  const assigned = new Set(tiles.flatMap((t) => t.tabs));
  const fresh = sessionIds.filter((id) => !assigned.has(id));
  if (fresh.length > 0) {
    tiles = tiles.map((t) =>
      t.id === activeTileId
        ? { ...t, tabs: [...t.tabs, ...fresh], activeTabId: fresh[fresh.length - 1] }
        : t,
    );
  }

  // 5. Focus follows activeSessionId: its tile becomes active, that tab selected.
  if (activeSessionId && present.has(activeSessionId)) {
    const host = tiles.find((t) => t.tabs.includes(activeSessionId));
    if (host) {
      activeTileId = host.id;
      tiles = tiles.map((t) =>
        t.id === host.id ? { ...t, activeTabId: activeSessionId } : t,
      );
    }
  }

  const next = withTiles({ ...layout, activeTileId }, tiles);
  return { ...next, activeTileId };
}
