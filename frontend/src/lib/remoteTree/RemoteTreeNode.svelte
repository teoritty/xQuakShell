<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import {
    CheckCircle2,
    ChevronDown,
    ChevronRight,
    Circle,
    Folder as FolderIcon,
    FolderOpen,
    Loader2,
    Monitor,
    Pencil,
    Plus,
    X,
    XCircle,
  } from 'lucide-svelte';
  import { pingColor, pingTooltip, tagColor } from './connectionDisplay';
  import { range } from './buildTree';
  import { isNodeEditing } from './dndGuards';
  import type { ConnectionStatus, DropZone, TreeNode } from './types';
  import './remoteTreeShared.css';

  export let node: TreeNode;
  export let selected = false;
  export let ariaSelected = false;
  export let draggable = true;
  export let dragOverDropZone: DropZone | null = null;
  export let dragOverTargetId: string | null = null;
  export let editingFolderId: string | null = null;
  export let editingConnId: string | null = null;
  export let editingFolderName = '';
  export let editingConnName = '';
  export let pingResults: Map<string, { reachable?: boolean; latencyMs?: number }> = new Map();
  export let sessionStatusByConnId: Map<string, ConnectionStatus> = new Map();
  export let selectedConnectionCount = 1;

  const dispatch = createEventDispatcher();

  $: isEditing = isNodeEditing(node, editingFolderId, editingConnId);
  $: dropTarget = dragOverDropZone === 'folder' && dragOverTargetId === node.id;
  $: dropBefore = dragOverDropZone === 'before' && dragOverTargetId === node.id;
  $: dropAfter = dragOverDropZone === 'after' && dragOverTargetId === node.id;
</script>

<div
  class="tree-node"
  class:folder={node.type === 'folder'}
  class:connection={node.type === 'connection'}
  class:selected
  class:drop-target={dropTarget}
  class:drop-target-before={dropBefore}
  class:drop-target-after={dropAfter}
  style="padding-left: calc({Math.min(8 + node.depth * 12, 100)}px * var(--ui-scale))"
  draggable={draggable && !isEditing}
  role="treeitem"
  aria-selected={ariaSelected}
  tabindex="0"
  on:dragstart={(e) => !isEditing && dispatch('dragstart', { event: e, node })}
  on:dragend={() => dispatch('dragend')}
  on:dragover={(e) => dispatch('dragover', { event: e, node })}
  on:dragenter={() => node.type === 'folder' && dispatch('dragenter', { node })}
  on:dragleave={() => node.type === 'folder' && dispatch('dragleave')}
  on:drop={(e) => dispatch('drop', { event: e, node })}
  on:click={(e) => dispatch('click', { event: e, node })}
  on:dblclick={() => dispatch('dblclick', { node })}
  on:contextmenu={(e) => dispatch('contextmenu', { event: e, node })}
  on:keydown={(e) => dispatch('keydown', { event: e, node })}
>
  {#each range(node.depth) as l}
    <span class="indent-guide" style="left: calc({8 + l * 12 + 7}px * var(--ui-scale))"></span>
  {/each}
  {#if node.type === 'folder'}
    <span
      class="folder-arrow"
      role="button"
      tabindex="-1"
      on:click|stopPropagation={() => dispatch('toggleFolder', { id: node.id })}
      on:keydown|stopPropagation={(e) => e.key === 'Enter' && dispatch('toggleFolder', { id: node.id })}
    >
      {#if node.expanded}<ChevronDown size={12} />{:else}<ChevronRight size={12} />{/if}
    </span>
    {#if editingFolderId === node.id}
      <input
        class="inline-input"
        bind:value={editingFolderName}
        on:mousedown|stopPropagation
        on:blur={() => dispatch('confirmRenameFolder')}
        on:keydown={(e) => {
          if (e.key === 'Enter') dispatch('confirmRenameFolder');
          if (e.key === 'Escape') dispatch('cancelRenameFolder');
        }}
      />
    {:else}
      <span class="folder-icon">
        {#if node.expanded}<FolderOpen size={14} />{:else}<FolderIcon size={14} />{/if}
      </span>
      <span class="node-name">{node.name}</span>
      <div class="folder-actions">
        <button class="micro-btn" on:click|stopPropagation={() => dispatch('newSubfolder', { folderId: node.id })} title="New subfolder">
          <Plus size={12} />
        </button>
        <button class="micro-btn" on:click|stopPropagation={() => dispatch('startRenameFolder', { folder: node.folder })} title="Rename">
          <Pencil size={12} />
        </button>
        <button class="micro-btn danger" on:click|stopPropagation={() => dispatch('deleteFolder', { folder: node.folder })} title="Delete">
          <X size={12} />
        </button>
      </div>
    {/if}
  {:else}
    <span class="ping-dot" style="background: {pingColor(pingResults, node.id)}" title={pingTooltip(pingResults, node.id)}></span>
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
  {/if}
</div>
