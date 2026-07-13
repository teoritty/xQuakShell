// frontend/src/lib/tiles/operations.ts
// User-gesture layout transitions. Pure: takes a TileLayout, returns a new one,
// always leaving it in a canonical arrangement. Never touches session lifecycle.

import type { TileLayout, TileGroup, Edge, Orientation } from './types';
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
  // At 2 tiles every edge is meaningful: a perpendicular drop subdivides into a
  // T (when the dragged tab's tile keeps other tabs), while a drop that empties
  // the source re-orients the two tiles onto the dropped edge's axis.
  if (n === 2) return ['left', 'right', 'top', 'bottom'];
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
  if (!source) return layout;
  if (!layout.tiles.some((t) => t.id === targetTileId)) return layout;
  if (!allowedEdges(layout, targetTileId).includes(edge)) return layout;

  // A lone-tab tile empties when its only tab is dragged out. That is a valid
  // gesture only at 2 tiles (it re-orients, keeping the count); elsewhere it
  // would just relocate a tile without changing the layout, so refuse it.
  const emptiesSource = source.tabs.length < 2;
  if (emptiesSource && n !== 2) return layout;

  const edgeAxis: Orientation = edge === 'left' || edge === 'right' ? 'h' : 'v';

  // Strip the moved session from its source, in place within the array.
  const base = layout.tiles.map((t) => (t.id === source.id ? removeTab(t, sessionId) : t));
  const moved: TileGroup = { id: newTileId(), tabs: [sessionId], activeTabId: sessionId };
  const target = base.find((t) => t.id === targetTileId)!;

  let tiles: TileGroup[];
  let orientation = layout.orientation;

  if (n === 1) {
    orientation = edgeAxis;
    tiles = isFirstHalf(edge) ? [moved, target] : [target, moved];
  } else if (n === 2) {
    if (emptiesSource) {
      // Re-orientation: the dragged tab's source tile empties, so the count
      // stays 2. Re-lay the surviving sibling + the moved tab along the dropped
      // edge's axis, with the moved tab sitting on the dropped edge.
      const sibling = base.find((t) => t.id !== source.id && t.tabs.length > 0)!;
      tiles = isFirstHalf(edge) ? [moved, sibling] : [sibling, moved];
      orientation = edgeAxis;
    } else {
      // Source keeps tabs → this grows to 3 tiles, canonical only as a T
      // (perpendicular subdivision). A same-axis drop has no canonical 3-tile
      // form, so ignore it.
      if (edgeAxis === layout.orientation) return layout;
      const other = base.find((t) => t.id !== targetTileId)!;
      const pair = isFirstHalf(edge) ? [moved, target] : [target, moved];
      tiles = [other, ...pair];
    }
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
