// frontend/src/stores/tileLayout.ts
// Reactive glue between the pure tile modules and the app. Placement-only:
// it reconciles the layout whenever the open tabs or the active one change, and
// exposes the user-gesture actions. It NEVER opens/closes sessions or surfaces.

import { writable, get } from 'svelte/store';
import { sessions, activeSessionId } from './appState';
import { surfaces } from './surfaceState';
import type { TileLayout, Edge } from '../lib/tiles/types';
import { emptyLayout } from '../lib/tiles/types';
import { reconcile } from '../lib/tiles/reconcile';
import { splitOut, moveTab, reorient, swapTiles } from '../lib/tiles/operations';

export const tileLayout = writable<TileLayout>(emptyLayout());

/**
 * The tile tab currently being dragged, or null. Set on dragstart and cleared on
 * dragend. Drop targets read it during `dragover` (where the DataTransfer payload
 * is not yet readable) to decide whether the gesture splits, re-orients or moves.
 */
export const activeTileDrag = writable<{ sessionId: string; sourceTileId: string } | null>(null);

// A tab is a session OR a plugin-owned surface (ADR-015). Surfaces are listed
// after sessions rather than merged by arrival time: order only decides where a
// batch of brand-new tabs lands inside one tile, and a surface is always the
// newer of the two anyway — it can only exist under a session that is already open.
function sync(): void {
  const ids = [
    ...get(sessions).map((s) => s.sessionId),
    ...get(surfaces).map((s) => s.surfaceId),
  ];
  const active = get(activeSessionId);
  tileLayout.update((l) => reconcile(l, ids, active));
}

// All three stores drive reconciliation. subscribe fires immediately, seeding
// the layout from whatever is already open. Leaving `surfaces` out is what made
// a plugin tab open into nothing: the surface existed, and reconcile stripped
// its id from every tile because it was not in the list it was given.
sessions.subscribe(sync);
surfaces.subscribe(sync);
activeSessionId.subscribe(sync);

export function splitOutTile(sessionId: string, targetTileId: string, edge: Edge): void {
  tileLayout.update((l) => splitOut(l, sessionId, targetTileId, edge));
}

export function moveTabToTile(sessionId: string, targetTileId: string): void {
  tileLayout.update((l) => moveTab(l, sessionId, targetTileId));
}

export function reorientTile(sessionId: string, edge: Edge): void {
  tileLayout.update((l) => reorient(l, sessionId, edge));
}

export function swapTilesById(sessionId: string, targetTileId: string): void {
  tileLayout.update((l) => swapTiles(l, sessionId, targetTileId));
}

export function setDivider(divider: 'main' | 'cross', ratio: number): void {
  tileLayout.update((l) => ({ ...l, dividers: { ...l.dividers, [divider]: ratio } }));
}
