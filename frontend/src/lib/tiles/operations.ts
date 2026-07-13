// frontend/src/lib/tiles/operations.ts
// User-gesture layout transitions. Pure: takes a TileLayout, returns a new one,
// always leaving it in a canonical arrangement. Never touches session lifecycle.

import type { TileLayout, TileGroup, Edge } from './types';
import { newTileId } from './types';
import { withTiles, removeTab } from './invariants';

export function tileOf(layout: TileLayout, sessionId: string): TileGroup | undefined {
  return layout.tiles.find((t) => t.tabs.includes(sessionId));
}

/** Which edge drops are legal on `targetTileId` given the current layout. */
export function allowedEdges(layout: TileLayout, targetTileId: string): Edge[] {
  const n = layout.tiles.length;
  if (n >= 4) return [];
  if (n === 1) return ['left', 'right', 'top', 'bottom'];
  if (n === 2) return layout.orientation === 'h' ? ['top', 'bottom'] : ['left', 'right'];
  // n === 3: only the full tile (slot 0), only along the cross axis
  if (layout.tiles[0]?.id !== targetTileId) return [];
  return layout.orientation === 'h' ? ['top', 'bottom'] : ['left', 'right'];
}

const isFirstHalf = (edge: Edge) => edge === 'left' || edge === 'top';

export function splitOut(
  layout: TileLayout,
  sessionId: string,
  targetTileId: string,
  edge: Edge,
): TileLayout {
  const n = layout.tiles.length;
  if (n >= 4) return layout;
  const source = tileOf(layout, sessionId);
  if (!source || source.tabs.length < 2) return layout; // need >=2 tabs to split
  if (!layout.tiles.some((t) => t.id === targetTileId)) return layout;
  if (!allowedEdges(layout, targetTileId).includes(edge)) return layout;

  // Strip the moved session from its source, in place within the array.
  const base = layout.tiles.map((t) => (t.id === source.id ? removeTab(t, sessionId) : t));
  const moved: TileGroup = { id: newTileId(), tabs: [sessionId], activeTabId: sessionId };
  const target = base.find((t) => t.id === targetTileId)!;

  let tiles: TileGroup[];
  let orientation = layout.orientation;

  if (n === 1) {
    orientation = edge === 'left' || edge === 'right' ? 'h' : 'v';
    tiles = isFirstHalf(edge) ? [moved, target] : [target, moved];
  } else if (n === 2) {
    // The dropped-on tile subdivides; the other tile becomes the full slot 0.
    const other = base.find((t) => t.id !== targetTileId)!;
    const pair = isFirstHalf(edge) ? [moved, target] : [target, moved];
    tiles = [other, ...pair];
  } else {
    // n === 3 -> 4. target is slot 0 (the full tile); split it across the cross axis.
    const full = target;
    const a = base[1];
    const b = base[2];
    const fullPair = isFirstHalf(edge) ? [moved, full] : [full, moved];
    tiles =
      layout.orientation === 'h'
        ? [fullPair[0], a, fullPair[1], b] // [TL, TR, BL, BR]
        : [fullPair[0], fullPair[1], a, b]; // [TL, TR, BL, BR]
  }
  return withTiles(layout, tiles, orientation);
}

/** Moves a tab into another existing tile (center drop). Collapses an emptied source. */
export function moveTab(layout: TileLayout, sessionId: string, targetTileId: string): TileLayout {
  const source = tileOf(layout, sessionId);
  if (!source || source.id === targetTileId) return layout;
  if (!layout.tiles.some((t) => t.id === targetTileId)) return layout;
  const tiles = layout.tiles
    .map((t) => (t.id === source.id ? removeTab(t, sessionId) : t))
    .map((t) =>
      t.id === targetTileId
        ? { ...t, tabs: [...t.tabs, sessionId], activeTabId: sessionId }
        : t,
    )
    .filter((t) => t.tabs.length > 0);
  return withTiles(layout, tiles);
}
