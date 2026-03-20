<script lang="ts">
  import { connections, selectedConnectionId } from '../stores/appState';
  import type { Connection } from '../stores/appState';
  const filteredConnections = connections;
  import { openSession } from '../stores/api';

  function selectConnection(id: string) {
    selectedConnectionId.set(id);
  }

  function handleDragStart(e: DragEvent, connection: Connection) {
    if (e.dataTransfer) {
      e.dataTransfer.setData('text/plain', JSON.stringify([connection.id]));
      e.dataTransfer.effectAllowed = 'move';
    }
  }

  async function connectDoubleClick(conn: Connection) {
    await openSession(conn.id);
  }
</script>

<div class="connection-list">
  <div class="panel-header">
    <span>Connections</span>
  </div>

  <div class="list-body">
    {#if $filteredConnections.length === 0}
      <div class="empty-message">No connections</div>
    {/if}

    {#each $filteredConnections as conn (conn.id)}
      <div
        class="connection-item"
        class:active={$selectedConnectionId === conn.id}
        on:click={() => selectConnection(conn.id)}
        on:dblclick={() => connectDoubleClick(conn)}
        on:keydown={(e) => e.key === 'Enter' && selectConnection(conn.id)}
        draggable="true"
        on:dragstart={(e) => handleDragStart(e, conn)}
        role="button"
        tabindex="0"
      >
        <span class="conn-icon">🖥️</span>
        <div class="conn-info">
          <div class="conn-name">{conn.name || 'Unnamed'}</div>
          <div class="conn-host">{conn.user}@{conn.host}:{conn.port}</div>
        </div>
      </div>
    {/each}
  </div>
</div>

<style>
  .connection-list {
    display: flex;
    flex-direction: column;
    border-bottom: 1px solid var(--border-color);
    flex: 1;
    min-height: 0;
  }

  .list-body {
    overflow-y: auto;
    flex: 1;
  }

  .empty-message {
    padding: 16px;
    text-align: center;
    color: var(--text-secondary);
    font-size: 12px;
    font-style: italic;
  }

  .connection-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 10px;
    cursor: pointer;
    user-select: none;
    transition: background 0.1s;
    border-bottom: 1px solid transparent;
  }

  .connection-item:hover {
    background: var(--bg-hover);
  }

  .connection-item.active {
    background: var(--bg-active);
    color: var(--text-bright);
  }

  .connection-item[draggable="true"] {
    cursor: grab;
  }

  .connection-item[draggable="true"]:active {
    cursor: grabbing;
  }

  .conn-icon {
    font-size: 16px;
    flex-shrink: 0;
  }

  .conn-info {
    flex: 1;
    min-width: 0;
    overflow: hidden;
  }

  .conn-name {
    font-size: 12px;
    font-weight: 500;
    color: var(--text-bright);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .conn-host {
    font-size: 11px;
    color: var(--text-secondary);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
