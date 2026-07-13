// frontend/src/lib/tiles/dragPayload.ts
// The DataTransfer contract for dragging a tile tab. A private MIME keeps this
// channel disjoint from the OS file-drop router (which keys off the 'Files'
// type), so the two drag paths never interfere.

export const TILE_TAB_MIME = 'application/x-xquak-tile-tab';

export interface TileTabPayload {
  sessionId: string;
  sourceTileId: string;
}

export function writeDragPayload(dt: DataTransfer, p: TileTabPayload): void {
  dt.setData(TILE_TAB_MIME, JSON.stringify(p));
  dt.effectAllowed = 'move';
}

export function readDragPayload(dt: DataTransfer): TileTabPayload | null {
  const raw = dt.getData(TILE_TAB_MIME);
  if (!raw) return null;
  try {
    const parsed = JSON.parse(raw);
    if (typeof parsed?.sessionId === 'string' && typeof parsed?.sourceTileId === 'string') {
      return parsed;
    }
  } catch {
    // fall through
  }
  return null;
}

export function isTileTabDrag(dt: DataTransfer | null): boolean {
  return !!dt && Array.from(dt.types).includes(TILE_TAB_MIME);
}
