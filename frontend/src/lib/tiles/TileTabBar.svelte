<!-- frontend/src/lib/tiles/TileTabBar.svelte -->
<script lang="ts">
  import { sessions, activeSessionId } from '../../stores/appState';
  import { closeSession } from '../../stores/api';
  import type { TileGroup } from './types';
  import { writeDragPayload } from './dragPayload';
  import { Loader2, CheckCircle2, XCircle, Circle, X } from 'lucide-svelte';

  export let tile: TileGroup;

  // The Session objects for this tile, in tab order.
  $: tabSessions = tile.tabs
    .map((id) => $sessions.find((s) => s.sessionId === id))
    .filter((s): s is NonNullable<typeof s> => !!s);

  function activate(sessionId: string) {
    activeSessionId.set(sessionId);
  }

  async function close(e: MouseEvent, sessionId: string) {
    e.stopPropagation();
    await closeSession(sessionId);
  }

  function onDragStart(e: DragEvent, sessionId: string) {
    if (!e.dataTransfer) return;
    writeDragPayload(e.dataTransfer, { sessionId, sourceTileId: tile.id });
  }
</script>

<div class="tile-tab-bar">
  {#each tabSessions as session (session.sessionId)}
    <div
      class="tab"
      class:active={tile.activeTabId === session.sessionId}
      draggable="true"
      on:dragstart={(e) => onDragStart(e, session.sessionId)}
      on:click={() => activate(session.sessionId)}
      on:keydown={(e) => e.key === 'Enter' && activate(session.sessionId)}
      role="tab"
      tabindex="0"
    >
      <span class="tab-state">
        {#if session.state === 'connecting'}
          <Loader2 size={11} />
        {:else if session.state === 'ready'}
          <CheckCircle2 size={11} style="color: #4caf50" />
        {:else if session.state === 'error'}
          <XCircle size={11} style="color: var(--danger)" />
        {:else}
          <Circle size={11} />
        {/if}
      </span>
      <span class="tab-name">{session.connectionName || 'Session'}</span>
      <button class="tab-close" on:click={(e) => close(e, session.sessionId)} title="Close session">
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
