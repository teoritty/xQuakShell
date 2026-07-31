<script lang="ts">
  // Folder row. Split out of RemoteTreeNode.svelte unchanged — same markup, same
  // classes, same events, same order. The wrapper <div class="tree-node"> stays
  // in RemoteTreeNode.svelte because it is identical for all row kinds.
  import { createEventDispatcher } from 'svelte';
  import { ChevronDown, ChevronRight, Folder as FolderIcon, FolderOpen, Pencil, Plus, X } from 'lucide-svelte';
  import type { TreeNode } from './types';

  export let node: TreeNode;
  export let editingFolderId: string | null = null;
  export let editingFolderName = '';

  const dispatch = createEventDispatcher();
</script>

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
