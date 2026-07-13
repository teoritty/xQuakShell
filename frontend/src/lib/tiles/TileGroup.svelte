<!-- frontend/src/lib/tiles/TileGroup.svelte -->
<script lang="ts">
  import type { TileGroup, Zone, Edge } from './types';
  import { sessions, activeSessionId } from '../../stores/appState';
  import SessionView from '../SessionView.svelte';
  import TileTabBar from './TileTabBar.svelte';
  import TileDropOverlay from './TileDropOverlay.svelte';
  import {
    tileLayout,
    activeTileDrag,
    splitOutTile,
    moveTabToTile,
    reorientTile,
    swapTilesById,
  } from '../../stores/tileLayout';
  import { splitEdges, reorientEdges, isLoneTab } from './operations';
  import { zoneAt } from './dropZones';
  import { isTileTabDrag, readDragPayload } from './dragPayload';

  export let tile: TileGroup;

  type DropIntent = 'move' | 'split' | 'reorient' | 'swap';

  let root: HTMLElement;
  let zone: Zone | null = null;
  let intent: DropIntent | null = null;

  $: tileSessions = tile.tabs
    .map((id) => $sessions.find((s) => s.sessionId === id))
    .filter((s): s is NonNullable<typeof s> => !!s);

  // Resolves what a drop at (x,y) would do, given the tile being dragged.
  //  - lone connection, edge      -> reorient the layout
  //  - lone connection, centre    -> swap this tile with the dragged tile
  //  - tab from a multi-tab tile, edge   -> split out a new tile
  //  - tab from a multi-tab tile, centre -> move the connection in as a tab
  function resolve(e: DragEvent): { zone: Zone; intent: DropIntent | null } {
    const drag = $activeTileDrag;
    if (!drag) return { zone: 'center', intent: null };
    const lone = isLoneTab($tileLayout, drag.sessionId);
    const edges: Edge[] = lone ? reorientEdges($tileLayout) : splitEdges($tileLayout, tile.id);
    const r = root.getBoundingClientRect();
    const z = zoneAt({ left: r.left, top: r.top, width: r.width, height: r.height }, e.clientX, e.clientY, edges);
    if (z === 'center') {
      // Centre only does something when dropping onto a DIFFERENT tile.
      if (drag.sourceTileId === tile.id) return { zone: z, intent: null };
      return { zone: z, intent: lone ? 'swap' : 'move' };
    }
    return { zone: z, intent: lone ? 'reorient' : 'split' };
  }

  function onDragOver(e: DragEvent) {
    if (!isTileTabDrag(e.dataTransfer)) return; // let OS file drops pass through
    e.preventDefault();
    const res = resolve(e);
    zone = res.intent ? res.zone : null;
    intent = res.intent;
  }

  function onDragLeave(e: DragEvent) {
    if (!e.relatedTarget || !root.contains(e.relatedTarget as Node)) {
      zone = null;
      intent = null;
    }
  }

  function onDrop(e: DragEvent) {
    if (!isTileTabDrag(e.dataTransfer)) return;
    e.preventDefault();
    const payload = readDragPayload(e.dataTransfer!);
    const z = zone;
    const act = intent;
    zone = null;
    intent = null;
    if (!payload || !act) return;
    if (act === 'move') {
      moveTabToTile(payload.sessionId, tile.id);
    } else if (act === 'swap') {
      swapTilesById(payload.sessionId, tile.id);
    } else if (act === 'reorient' && z && z !== 'center') {
      reorientTile(payload.sessionId, z);
    } else if (act === 'split' && z && z !== 'center') {
      splitOutTile(payload.sessionId, tile.id, z);
    }
  }

  function focusTile() {
    // Clicking anywhere in the tile focuses its active tab (keeps global
    // activeSessionId — and thus activeTileId — in sync).
    if (tile.activeTabId) activeSessionId.set(tile.activeTabId);
  }
</script>

<div
  class="tile-group"
  bind:this={root}
  on:dragover={onDragOver}
  on:dragleave={onDragLeave}
  on:drop={onDrop}
  on:mousedown={focusTile}
>
  <TileTabBar {tile} />
  <div class="tile-body">
    {#each tileSessions as session (session.sessionId)}
      <SessionView {session} active={tile.activeTabId === session.sessionId} />
    {/each}
  </div>
  <TileDropOverlay {zone} {intent} />
</div>

<style>
  .tile-group {
    position: relative;
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
    overflow: hidden;
    border: 1px solid var(--border-color);
  }
  .tile-body {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }
</style>
