<!-- frontend/src/lib/tiles/TileGroup.svelte -->
<script lang="ts">
  import type { TileGroup, Zone } from './types';
  import { sessions, activeSessionId } from '../../stores/appState';
  import SessionView from '../SessionView.svelte';
  import TileTabBar from './TileTabBar.svelte';
  import TileDropOverlay from './TileDropOverlay.svelte';
  import { tileLayout, splitOutTile, moveTabToTile } from '../../stores/tileLayout';
  import { allowedEdges } from './operations';
  import { zoneAt } from './dropZones';
  import { isTileTabDrag, readDragPayload } from './dragPayload';

  export let tile: TileGroup;

  let root: HTMLElement;
  let zone: Zone | null = null;

  $: tileSessions = tile.tabs
    .map((id) => $sessions.find((s) => s.sessionId === id))
    .filter((s): s is NonNullable<typeof s> => !!s);

  function computeZone(e: DragEvent): Zone {
    const r = root.getBoundingClientRect();
    const allowed = allowedEdges($tileLayout, tile.id);
    return zoneAt(
      { left: r.left, top: r.top, width: r.width, height: r.height },
      e.clientX,
      e.clientY,
      allowed,
    );
  }

  function onDragOver(e: DragEvent) {
    if (!isTileTabDrag(e.dataTransfer)) return; // let OS file drops pass through
    e.preventDefault();
    zone = computeZone(e);
  }

  function onDragLeave(e: DragEvent) {
    if (!e.relatedTarget || !root.contains(e.relatedTarget as Node)) zone = null;
  }

  function onDrop(e: DragEvent) {
    if (!isTileTabDrag(e.dataTransfer)) return;
    e.preventDefault();
    const payload = readDragPayload(e.dataTransfer!);
    const target = zone;
    zone = null;
    if (!payload || !target) return;
    if (target === 'center') {
      moveTabToTile(payload.sessionId, tile.id);
    } else {
      splitOutTile(payload.sessionId, tile.id, target);
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
  <TileDropOverlay {zone} />
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
