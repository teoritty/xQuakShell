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
  let tabBarEl: HTMLElement;
  let zone: Zone | null = null;
  let intent: DropIntent | null = null;
  // True while a drag hovers this tile's tab bar (a merge/add-as-tab target).
  let mergeBar = false;

  $: tileSessions = tile.tabs
    .map((id) => $sessions.find((s) => s.sessionId === id))
    .filter((s): s is NonNullable<typeof s> => !!s);

  function inRect(el: HTMLElement | undefined, x: number, y: number): boolean {
    if (!el) return false;
    const r = el.getBoundingClientRect();
    return x >= r.left && x <= r.right && y >= r.top && y <= r.bottom;
  }

  // Resolves what a drop at (x,y) would do, given the tile being dragged.
  //  - over another tile's TAB BAR      -> merge: add the connection as a tab
  //  - lone connection, body edge       -> reorient the layout
  //  - lone connection, body centre     -> swap this tile with the dragged tile
  //  - tab from a multi-tab tile, edge   -> split out a new tile
  //  - tab from a multi-tab tile, centre -> move the connection in as a tab
  function resolve(e: DragEvent): { zone: Zone; intent: DropIntent | null; mergeBar: boolean } {
    const drag = $activeTileDrag;
    if (!drag) return { zone: 'center', intent: null, mergeBar: false };
    const differentTile = drag.sourceTileId !== tile.id;

    // Dropping onto another tile's tab bar merges the connection into it as a tab.
    if (differentTile && inRect(tabBarEl, e.clientX, e.clientY)) {
      return { zone: 'center', intent: 'move', mergeBar: true };
    }

    const lone = isLoneTab($tileLayout, drag.sessionId);
    const edges: Edge[] = lone ? reorientEdges($tileLayout) : splitEdges($tileLayout, tile.id);
    const r = root.getBoundingClientRect();
    const z = zoneAt({ left: r.left, top: r.top, width: r.width, height: r.height }, e.clientX, e.clientY, edges);
    if (z === 'center') {
      // Body centre only does something when dropping onto a DIFFERENT tile.
      if (!differentTile) return { zone: z, intent: null, mergeBar: false };
      return { zone: z, intent: lone ? 'swap' : 'move', mergeBar: false };
    }
    return { zone: z, intent: lone ? 'reorient' : 'split', mergeBar: false };
  }

  function onDragOver(e: DragEvent) {
    if (!isTileTabDrag(e.dataTransfer)) return; // let OS file drops pass through
    e.preventDefault();
    const res = resolve(e);
    zone = res.intent ? res.zone : null;
    intent = res.intent;
    mergeBar = res.mergeBar;
  }

  function onDragLeave(e: DragEvent) {
    if (!e.relatedTarget || !root.contains(e.relatedTarget as Node)) {
      zone = null;
      intent = null;
      mergeBar = false;
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
    mergeBar = false;
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
  <div class="tile-chrome" class:merge-target={mergeBar} bind:this={tabBarEl}>
    <TileTabBar {tile} />
  </div>
  <div class="tile-body">
    {#each tileSessions as session (session.sessionId)}
      <SessionView {session} active={tile.activeTabId === session.sessionId} />
    {/each}
  </div>
  <TileDropOverlay zone={mergeBar ? null : zone} intent={mergeBar ? null : intent} />
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
  .tile-chrome {
    display: flex;
    flex-direction: column;
    flex-shrink: 0;
    position: relative;
  }
  /* Highlight the tab bar when a drag hovers it: dropping here adds the
     connection to this tile as a tab. */
  .tile-chrome.merge-target {
    outline: 2px solid var(--accent);
    outline-offset: -2px;
    background: color-mix(in srgb, var(--accent) 22%, transparent);
  }
  .tile-body {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }
</style>
