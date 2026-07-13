// frontend/src/stores/tileLayout.ts
// Reactive glue between the pure tile modules and the app. Placement-only:
// it reconciles the layout whenever sessions or the active session change, and
// exposes the user-gesture actions. It NEVER opens/closes sessions.

import { writable, get } from 'svelte/store';
import { sessions, activeSessionId } from './appState';
import type { TileLayout, Edge } from '../lib/tiles/types';
import { emptyLayout } from '../lib/tiles/types';
import { reconcile } from '../lib/tiles/reconcile';
import { splitOut, moveTab, reorient } from '../lib/tiles/operations';

export const tileLayout = writable<TileLayout>(emptyLayout());

/**
 * The tile tab currently being dragged, or null. Set on dragstart and cleared on
 * dragend. Drop targets read it during `dragover` (where the DataTransfer payload
 * is not yet readable) to decide whether the gesture splits, re-orients or moves.
 */
export const activeTileDrag = writable<{ sessionId: string; sourceTileId: string } | null>(null);

function sync(): void {
  const ids = get(sessions).map((s) => s.sessionId);
  const active = get(activeSessionId);
  tileLayout.update((l) => reconcile(l, ids, active));
}

// Both stores drive reconciliation. subscribe fires immediately, seeding the
// layout from whatever sessions already exist.
sessions.subscribe(sync);
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

export function setDivider(divider: 'main' | 'cross', ratio: number): void {
  tileLayout.update((l) => ({ ...l, dividers: { ...l.dividers, [divider]: ratio } }));
}
