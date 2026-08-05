<!-- frontend/src/lib/tiles/TileTabBar.svelte -->
<script lang="ts">
  import { sessions, activeSessionId } from '../../stores/appState';
  import {
    surfaces,
    resolveTab,
    closeTab,
    tabTitle,
    tabState,
    type Tab,
  } from '../../stores/surfaceState';
  import type { TileGroup } from './types';
  import { writeDragPayload } from './dragPayload';
  import { activeTileDrag } from '../../stores/tileLayout';
  import { Loader2, CheckCircle2, XCircle, Circle, X } from 'lucide-svelte';

  export let tile: TileGroup;

  // The tabs of this tile in order, each resolved to the session or plugin surface behind it
  // (ADR-015). Both stores are referenced so the lookup re-runs when either changes.
  $: tabs = ($sessions, $surfaces, tile.tabs
    .map((id) => ({ id, tab: resolveTab(id) }))
    .filter((e): e is { id: string; tab: NonNullable<Tab> } => !!e.tab));

  function activate(sessionId: string) {
    activeSessionId.set(sessionId);
  }

  // Closing routes on what the id names — an SSH session or a plugin's tab. The tab bar itself
  // must not know the difference, which is why the routing lives in the store.
  async function close(e: MouseEvent, tabId: string) {
    e.stopPropagation();
    await closeTab(tabId);
  }

  function onDragStart(e: DragEvent, sessionId: string) {
    if (!e.dataTransfer) return;
    writeDragPayload(e.dataTransfer, { sessionId, sourceTileId: tile.id });
    activeTileDrag.set({ sessionId, sourceTileId: tile.id });
  }

  function onDragEnd() {
    activeTileDrag.set(null);
  }
</script>

<div class="tile-tab-bar">
  {#each tabs as entry (entry.id)}
    <div
      class="tab"
      class:active={tile.activeTabId === entry.id}
      draggable="true"
      on:dragstart={(e) => onDragStart(e, entry.id)}
      on:dragend={onDragEnd}
      on:click={() => activate(entry.id)}
      on:keydown={(e) => e.key === 'Enter' && activate(entry.id)}
      role="tab"
      tabindex="0"
    >
      <span class="tab-state">
        {#if tabState(entry.tab) === 'connecting'}
          <Loader2 size={11} />
        {:else if tabState(entry.tab) === 'ready'}
          <CheckCircle2 size={11} style="color: #4caf50" />
        {:else if tabState(entry.tab) === 'error'}
          <XCircle size={11} style="color: var(--danger)" />
        {:else}
          <Circle size={11} />
        {/if}
      </span>
      <span class="tab-name">{tabTitle(entry.tab)}</span>
      <button class="tab-close" on:click={(e) => close(e, entry.id)} title="Close tab">
        <X size={11} />
      </button>
    </div>
  {/each}
</div>

<style>
  .tile-tab-bar {
    display: flex;
    align-items: stretch;
    background: var(--bg-tertiary);
    border-bottom: 1px solid var(--border-color);
    overflow-x: auto;
    flex-shrink: 0;
    min-height: 30px;
  }
  .tab {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 0 10px;
    min-width: 100px;
    max-width: 180px;
    cursor: pointer;
    user-select: none;
    background: var(--tab-inactive-bg);
    border-right: 1px solid var(--border-color);
    transition: background 0.1s;
    font-size: 12px;
  }
  .tab:hover { background: var(--bg-hover); }
  .tab.active {
    background: var(--tab-active-bg);
    border-bottom: 2px solid var(--accent);
    color: var(--text-bright);
  }
  .tab-state { display: inline-flex; align-items: center; flex-shrink: 0; color: var(--text-secondary); }
  .tab-name { flex: 1; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .tab-close {
    background: transparent; color: var(--text-secondary); padding: 0 3px;
    display: none; align-items: center; border: none; cursor: pointer; flex-shrink: 0;
  }
  .tab:hover .tab-close { display: inline-flex; }
  .tab-close:hover { color: var(--danger); }
</style>
