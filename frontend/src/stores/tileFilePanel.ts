// frontend/src/stores/tileFilePanel.ts
// Per-tile UI state: which tiles currently have their file browser column
// collapsed. Kept separate from the pure tile geometry model (lib/tiles/*) —
// this is a view preference keyed by tile id, not placement data.

import { writable } from 'svelte/store';
import { tileLayout } from './tileLayout';

/** Ids of tiles whose right-hand file column is collapsed. */
export const collapsedTileFilePanels = writable<Set<string>>(new Set());

export function toggleTileFilePanel(tileId: string): void {
  collapsedTileFilePanels.update((set) => {
    const next = new Set(set);
    if (next.has(tileId)) next.delete(tileId);
    else next.add(tileId);
    return next;
  });
}

// Drop ids for tiles that no longer exist so the set can't grow unbounded across
// a long session. Tile ids are never reused, so this is purely housekeeping.
tileLayout.subscribe((layout) => {
  const live = new Set(layout.tiles.map((t) => t.id));
  collapsedTileFilePanels.update((set) => {
    let changed = false;
    const next = new Set<string>();
    for (const id of set) {
      if (live.has(id)) next.add(id);
      else changed = true;
    }
    return changed ? next : set;
  });
});
