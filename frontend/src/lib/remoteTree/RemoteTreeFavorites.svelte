<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { CheckCircle2, Circle, Loader2, Monitor, XCircle } from 'lucide-svelte';
  import { pingColor, pingTooltip } from './connectionDisplay';
  import type { Connection } from '../../stores/appState';
  import type { ConnectionStatus } from './types';
  import './remoteTreeShared.css';

  export let connections: Connection[] = [];
  export let selectedPaths: Set<string> = new Set();
  export let pingResults: Map<string, { reachable?: boolean; latencyMs?: number }> = new Map();
  export let sessionStatusByConnId: Map<string, ConnectionStatus> = new Map();

  const dispatch = createEventDispatcher();
</script>

{#if connections.length > 0}
  <div class="favorites-section">
    <div class="favorites-header">Favorites</div>
    {#each connections as conn (conn.id)}
      <div
        class="tree-node connection favorite-node"
        class:selected={selectedPaths.has(conn.id)}
        style="padding-left: calc(8px * var(--ui-scale))"
        role="treeitem"
        aria-selected={selectedPaths.has(conn.id)}
        tabindex="0"
        on:click={(e) => dispatch('select', { id: conn.id, event: e })}
        on:dblclick={() => dispatch('open', { connection: conn })}
        on:contextmenu={(e) =>
          dispatch('contextmenu', {
            event: e,
            node: { type: 'connection', id: conn.id, name: conn.name, depth: 0, parentId: '', connection: conn },
          })}
        on:keydown={(e) => e.key === 'Enter' && dispatch('open', { connection: conn })}
      >
        <span class="ping-dot" style="background: {pingColor(pingResults, conn.id)}" title={pingTooltip(pingResults, conn.id)}></span>
        <span class="conn-icon"><Monitor size={14} /></span>
        {#if sessionStatusByConnId.get(conn.id)}
          {@const status = sessionStatusByConnId.get(conn.id) ?? 'disconnected'}
          <span class="conn-status" class:active={status === 'active'} class:connecting={status === 'connecting'} class:error={status === 'error'} title={status}>
            {#if status === 'active'}<CheckCircle2 size={10} />
            {:else if status === 'connecting'}<span class="spinning"><Loader2 size={10} /></span>
            {:else if status === 'error'}<XCircle size={10} />
            {:else}<Circle size={10} />{/if}
          </span>
        {/if}
        <span class="node-name">{conn.name}</span>
      </div>
    {/each}
  </div>
{/if}
