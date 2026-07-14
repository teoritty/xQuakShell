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
  import { connectionProtocols } from '../../actions/protocolActions';
  import { hasFilePanel } from '../filePanelCapability';
  import { collapsedTileFilePanels, toggleTileFilePanel } from '../../stores/tileFilePanel';
  import { Combine, PanelRightClose, PanelRightOpen } from 'lucide-svelte';

  export let tile: TileGroup;

  type DropIntent = 'merge' | 'split' | 'reorient' | 'swap';

  let root: HTMLElement;
  let tabBarEl: HTMLElement;
  let zone: Zone | null = null;
  let intent: DropIntent | null = null;
  // True while a drag hovers this tile's tab bar (a merge/add-as-tab target).
  let mergeBar = false;

  $: tileSessions = tile.tabs
    .map((id) => $sessions.find((s) => s.sessionId === id))
    .filter((s): s is NonNullable<typeof s> => !!s);

  // Per-tile file-panel collapse state and the button that toggles it. The button
  // shows only when the tile's active connection actually has a file browser.
  $: collapsed = $collapsedTileFilePanels.has(tile.id);
  $: activeSession = $sessions.find((s) => s.sessionId === tile.activeTabId);
  $: showFilesToggle = !!activeSession && hasFilePanel(activeSession, $connectionProtocols);

  function inRect(el: HTMLElement | undefined, x: number, y: number): boolean {
    if (!el) return false;
    const r = el.getBoundingClientRect();
    return x >= r.left && x <= r.right && y >= r.top && y <= r.bottom;
  }

  // Resolves what a drop at (x,y) would do, given the tile being dragged.
  // Merging is ALWAYS the tab bar, for every configuration; the body is only ever
  // spatial (swap / reorient / split):
  //  - over another tile's TAB BAR       -> merge: add the connection as a tab
  //  - body centre, different tile        -> swap the two WHOLE tiles' positions
  //    (works from any tile — the dragged connection's whole tile is swapped)
  //  - lone connection, body edge         -> reorient the layout
  //  - tab from a multi-tab tile, body edge -> split out a new tile
  function resolve(e: DragEvent): { zone: Zone; intent: DropIntent | null; mergeBar: boolean } {
    const drag = $activeTileDrag;
    if (!drag) return { zone: 'center', intent: null, mergeBar: false };
    const differentTile = drag.sourceTileId !== tile.id;

    // Dropping onto another tile's tab bar merges the connection into it as a tab.
    if (differentTile && inRect(tabBarEl, e.clientX, e.clientY)) {
      return { zone: 'center', intent: 'merge', mergeBar: true };
    }

    const lone = isLoneTab($tileLayout, drag.sessionId);
    const edges: Edge[] = lone ? reorientEdges($tileLayout) : splitEdges($tileLayout, tile.id);
    const r = root.getBoundingClientRect();
    const z = zoneAt({ left: r.left, top: r.top, width: r.width, height: r.height }, e.clientX, e.clientY, edges);
    if (z === 'center') {
      // Body centre swaps the dragged connection's whole tile with this tile. Any
      // tile can be swapped with any other, regardless of how many tabs it holds.
      if (!differentTile) return { zone: z, intent: null, mergeBar: false };
      return { zone: z, intent: 'swap', mergeBar: false };
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
    if (act === 'merge') {
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
    <div class="tile-tabs">
      <TileTabBar {tile} />
    </div>
    {#if showFilesToggle}
      <button
        class="tile-action"
        title={collapsed ? 'Show files' : 'Hide files'}
        on:click|stopPropagation={() => toggleTileFilePanel(tile.id)}
      >
        {#if collapsed}<PanelRightOpen size={18} />{:else}<PanelRightClose size={18} />{/if}
      </button>
    {/if}
    {#if mergeBar}
      <div class="merge-hint" aria-hidden="true">
        <Combine size={15} />
        <span>Combine into this tile</span>
      </div>
    {/if}
  </div>
  <div class="tile-body">
    {#each tileSessions as session (session.sessionId)}
      <SessionView
        {session}
        active={tile.activeTabId === session.sessionId}
        filesCollapsed={collapsed}
      />
    {/each}
  </div>
  <TileDropOverlay
    zone={intent === 'merge' ? null : zone}
    intent={intent === 'merge' ? null : intent}
  />
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
    flex-direction: row;
    align-items: stretch;
    flex-shrink: 0;
    position: relative;
  }
  .tile-tabs {
    display: flex;
    flex: 1 1 auto;
    min-width: 0;
  }
  .tile-tabs :global(.tile-tab-bar) {
    flex: 1;
    min-width: 0;
  }
  /* Per-tile file-panel toggle; matches the tab bar so the top bar reads as one strip. */
  .tile-action {
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    width: 34px;
    border: none;
    border-bottom: 1px solid var(--border-color);
    background: var(--bg-tertiary);
    color: var(--text-secondary);
    cursor: pointer;
    transition: background 0.1s, color 0.1s;
  }
  .tile-action:hover {
    color: var(--accent);
    background: var(--bg-hover);
  }
  /* Highlight the whole tab bar when a drag hovers it: dropping here combines the
     connection into this tile as a tab. */
  .tile-chrome.merge-target {
    outline: 2px solid var(--accent);
    outline-offset: -2px;
  }
  .merge-hint {
    position: absolute;
    inset: 0;
    z-index: 11;
    display: flex;
    align-items: center;
    justify-content: center;
    gap: 7px;
    pointer-events: none;
    background: color-mix(in srgb, var(--accent) 82%, transparent);
    color: var(--text-bright, #fff);
    font-size: 12px;
    font-weight: 600;
    letter-spacing: 0.2px;
    white-space: nowrap;
    overflow: hidden;
  }
  .tile-body {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }
</style>
