// frontend/src/lib/tiles/types.ts
// Pure data model for the tiled connection layout. No Svelte, no session
// lifecycle — a tile only records WHICH sessions are shown WHERE.

export type Orientation = 'h' | 'v';
export type Edge = 'left' | 'right' | 'top' | 'bottom';
export type Zone = Edge | 'center';

/** One tile = a group of session tabs with its own mini tab bar. */
export interface TileGroup {
  id: string;
  tabs: string[];       // sessionIds, in tab order
  activeTabId: string;  // '' when the tile is empty
}

/** Ratios (0..1) for the up-to-two resizable dividers of a layout. */
export interface Dividers {
  main: number;
  cross: number;
}

/** The whole layout. `tiles` length is 1..4; index === canonical slot. */
export interface TileLayout {
  tiles: TileGroup[];
  orientation: Orientation; // fixed at the 1->2 split, retained thereafter
  activeTileId: string;     // where new sessions land / which tile is focused
  dividers: Dividers;
}

let seq = 0;
export function newTileId(): string {
  return `tile-${Date.now().toString(36)}-${seq++}`;
}

export function emptyTile(id: string = newTileId()): TileGroup {
  return { id, tabs: [], activeTabId: '' };
}

export function emptyLayout(): TileLayout {
  const tile = emptyTile();
  return {
    tiles: [tile],
    orientation: 'h',
    activeTileId: tile.id,
    dividers: { main: 0.5, cross: 0.5 },
  };
}
