<script lang="ts">
  // Connection row. Split out of RemoteTreeNode.svelte with its behaviour
  // unchanged: the same ping slot (spinner until the first result), the same
  // session-status glyphs, the same rename input, tag chips and hover actions.
  // pingCharacterization.test.ts pins the colours and tooltips this renders.
  //
  // The one addition is the discovery arrow, and it appears ONLY when the
  // connection can actually have a subtree. A connection with no session shows
  // exactly what it showed before.
  import { createEventDispatcher } from 'svelte';
  import {
    CheckCircle2,
    ChevronDown,
    ChevronRight,
    Circle,
    Loader2,
    Monitor,
    Pencil,
    X,
    XCircle,
  } from 'lucide-svelte';
  import StatusDot from './StatusDot.svelte';
  import { hasPingResult, pingStatus, tagColor } from './connectionDisplay';
  import type { ConnectionStatus, TreeNode } from './types';

  export let node: TreeNode;
  export let selected = false;
  export let editingConnId: string | null = null;
  export let editingConnName = '';
  export let pingResults: Map<string, { reachable?: boolean; latencyMs?: number }> = new Map();
  export let sessionStatusByConnId: Map<string, ConnectionStatus> = new Map();
  export let selectedConnectionCount = 1;
  /** True when discovery is possible here at all (the connection has a session). */
  export let discoveryAvailable = false;
  export let discoveryExpanded = false;

  const dispatch = createEventDispatcher();
</script>

{#if discoveryAvailable}
  <span
    class="folder-arrow"
    role="button"
    tabindex="-1"
    title={discoveryExpanded ? 'Hide discovered resources' : 'Show discovered resources'}
    on:click|stopPropagation={() => dispatch('toggleDiscoveryRoot', { connectionId: node.id })}
    on:keydown|stopPropagation={(e) => e.key === 'Enter' && dispatch('toggleDiscoveryRoot', { connectionId: node.id })}
  >
    {#if discoveryExpanded}<ChevronDown size={12} />{:else}<ChevronRight size={12} />{/if}
  </span>
{/if}
{#if hasPingResult(pingResults, node.id)}
  <StatusDot status={pingStatus(pingResults, node.id)} />
{:else}
  <span class="ping-spinner" title="Pinging…"><Loader2 size={10} /></span>
{/if}
<span class="conn-icon"><Monitor size={14} /></span>
{#if sessionStatusByConnId.get(node.id)}
  {@const status = sessionStatusByConnId.get(node.id) ?? 'disconnected'}
  <span class="conn-status" class:active={status === 'active'} class:connecting={status === 'connecting'} class:error={status === 'error'} title={status}>
    {#if status === 'active'}<CheckCircle2 size={10} />
    {:else if status === 'connecting'}<span class="spinning"><Loader2 size={10} /></span>
    {:else if status === 'error'}<XCircle size={10} />
    {:else}<Circle size={10} />{/if}
  </span>
{/if}
{#if editingConnId === node.id}
  <input
    class="inline-input"
    bind:value={editingConnName}
    on:mousedown|stopPropagation
    on:blur={() => dispatch('confirmRenameConnection')}
    on:keydown={(e) => {
      if (e.key === 'Enter') dispatch('confirmRenameConnection');
      if (e.key === 'Escape') dispatch('cancelRenameConnection');
    }}
  />
{:else}
  <span class="node-name">{node.name}</span>
  {#if node.tags && node.tags.length > 0}
    <span class="tag-chips" title={node.tags.join(', ')}>
      {#each node.tags.slice(0, 2) as tag}
        <span class="tag-chip" style="background: {tagColor(tag)}">{tag}</span>
      {/each}
      {#if node.tags.length > 2}
        <span class="tag-more">+{node.tags.length - 2}</span>
      {/if}
    </span>
  {/if}
  <div class="conn-actions">
    <button class="micro-btn" on:click|stopPropagation={() => dispatch('startRenameConnection', { connection: node.connection })} title="Rename">
      <Pencil size={12} />
    </button>
    <button
      class="micro-btn danger"
      on:click|stopPropagation={() => dispatch('deleteConnection', { connection: node.connection, multi: selectedConnectionCount > 1 && selected })}
      title="Delete"
    >
      <X size={12} />
    </button>
  </div>
{/if}
