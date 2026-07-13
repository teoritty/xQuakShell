// frontend/src/lib/tiles/operations.ts
// User-gesture layout transitions. Pure: takes a TileLayout, returns a new one,
// always leaving it in a canonical arrangement. Never touches session lifecycle.
//
// Three distinct gestures, chosen by the caller from the drag context:
//  - moveTab   : drop on a tile's centre -> the connection joins that tile as a tab.
//  - splitOut  : drop on an edge while the source tile has OTHER tabs -> a new tile
//                is created (this is the only way the tile count grows).
//  - reorient  : drop on an edge while the source tile holds only that one
//                connection -> the whole tiles are re-laid (swap / flip), the tile
//                count is unchanged. Reuses the existing tile objects, so nothing
//                remounts.

import type { TileLayout, TileGroup, Edge, Orientation } from './types';
import { newTileId } from './types';
import { withTiles, removeTab } from './invariants';

export function tileOf(layout: TileLayout, sessionId: string): TileGroup | undefined {
  return layout.tiles.find((t) => t.tabs.includes(sessionId));
}

const isFirstHalf = (edge: Edge) => edge === 'left' || edge === 'top';
const axisOf = (edge: Edge): Orientation => (edge === 'left' || edge === 'right' ? 'h' : 'v');

/** True when the session is the only tab in its tile (dragging it moves a whole tile). */
export function isLoneTab(layout: TileLayout, sessionId: string): boolean {
  const t = tileOf(layout, sessionId);
  return !!t && t.tabs.length <= 1;
}

/**
 * Edges on `targetTileId` where dropping a tab from a MULTI-tab tile creates a
 * new tile. This is the only growth path, so the geometry stays canonical:
 * n=1 -> any edge; n=2 -> the perpendicular edges (forming a T); n=3 -> only the
 * full tile along its cross axis; n>=4 -> none (hard cap).
 */
export function splitEdges(layout: TileLayout, targetTileId: string): Edge[] {
  const n = layout.tiles.length;
  if (n >= 4) return [];
  if (n === 1) return ['left', 'right', 'top', 'bottom'];
  if (n === 2) return layout.orientation === 'h' ? ['top', 'bottom'] : ['left', 'right'];
  if (layout.tiles[0]?.id !== targetTileId) return [];
  return layout.orientation === 'h' ? ['top', 'bottom'] : ['left', 'right'];
}

/**
 * Edges that re-orient the layout when a lone-connection tile is dragged. Never
 * changes the tile count. At 2 tiles any edge works (a same-axis edge swaps the
 * two tiles, a cross-axis edge flips between side-by-side and stacked); at 3
 * tiles a cross-axis edge flips the T's orientation. A single tile has nothing
 * to re-orient and a 2x2 is fixed.
 */
export function reorientEdges(layout: TileLayout): Edge[] {
  const n = layout.tiles.length;
  if (n === 2 || n === 3) return ['left', 'right', 'top', 'bottom'];
  return [];
}

export function splitOut(
  layout: TileLayout,
  sessionId: string,
  targetTileId: string,
  edge: Edge,
): TileLayout {
  const n = layout.tiles.length;
  if (n >= 4) return layout;
  const source = tileOf(layout, sessionId);
  if (!source || source.tabs.length < 2) return layout; // need >=2 tabs to spawn a tile
  if (!layout.tiles.some((t) => t.id === targetTileId)) return layout;
  if (!splitEdges(layout, targetTileId).includes(edge)) return layout;

  // Strip the moved session from its source, in place within the array.
  const base = layout.tiles.map((t) => (t.id === source.id ? removeTab(t, sessionId) : t));
  const moved: TileGroup = { id: newTileId(), tabs: [sessionId], activeTabId: sessionId };
  const target = base.find((t) => t.id === targetTileId)!;

  let tiles: TileGroup[];
  let orientation = layout.orientation;

  if (n === 1) {
    orientation = axisOf(edge);
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

/**
 * Re-orients the layout by dragging a lone-connection tile to an edge. Keeps the
 * same tiles (and their ids, so nothing remounts) — only their arrangement
 * changes. No-op unless the edge is a `reorientEdges` edge.
 */
export function reorient(layout: TileLayout, sessionId: string, edge: Edge): TileLayout {
  const n = layout.tiles.length;
  const source = tileOf(layout, sessionId);
  if (!source) return layout;
  if (!reorientEdges(layout).includes(edge)) return layout;
  const targetAxis = axisOf(edge);

  if (n === 2) {
    const sibling = layout.tiles.find((t) => t.id !== source.id);
    if (!sibling) return layout;
    // Dragged tile sits on the dropped edge; the sibling takes the other half.
    const tiles = isFirstHalf(edge) ? [source, sibling] : [sibling, source];
    return withTiles(layout, tiles, targetAxis);
  }
  // n === 3: flip the T onto the dropped axis (a same-axis drop is a no-op).
  if (targetAxis === layout.orientation) return layout;
  return withTiles(layout, layout.tiles, targetAxis);
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
