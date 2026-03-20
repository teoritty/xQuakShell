<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { FolderPlus, Trash2, FilePlus, Pencil } from 'lucide-svelte';

  export let x = 0;
  export let y = 0;
  export let show = false;
  export let isDir = false;
  export let isEmptyArea = false;

  const dispatch = createEventDispatcher<{
    delete: void;
    newFolder: void;
    newFile: void;
    edit: void;
  }>();

  function handleDelete() {
    dispatch('delete');
  }

  function handleNewFolder() {
    dispatch('newFolder');
  }

  function handleNewFile() {
    dispatch('newFile');
  }

  function handleEdit() {
    dispatch('edit');
  }
</script>

{#if show}
  <div
    class="context-menu"
    style="left: {x}px; top: {y}px"
    role="menu"
    on:click|stopPropagation
  >
    {#if !isEmptyArea}
      <button class="menu-item danger" on:click={handleDelete} role="menuitem">
        <Trash2 size={12} />
        <span>Delete</span>
      </button>
      {#if !isDir}
        <button class="menu-item" on:click={handleEdit} role="menuitem">
          <Pencil size={12} />
          <span>Edit</span>
        </button>
      {/if}
    {/if}
    {#if isDir || isEmptyArea}
      <button class="menu-item" on:click={handleNewFolder} role="menuitem">
        <FolderPlus size={12} />
        <span>New Folder</span>
      </button>
      {#if isDir}
        <button class="menu-item" on:click={handleNewFile} role="menuitem">
          <FilePlus size={12} />
          <span>New File</span>
        </button>
      {/if}
    {/if}
  </div>
{/if}

<style>
  .context-menu {
    position: fixed;
    z-index: 1000;
    min-width: 140px;
    padding: 4px 0;
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 6px;
    box-shadow: 0 4px 12px rgba(0, 0, 0, 0.3);
  }

  .menu-item {
    display: flex;
    align-items: center;
    gap: 8px;
    width: 100%;
    padding: 6px 12px;
    border: none;
    background: transparent;
    color: var(--text-primary);
    font-size: 12px;
    cursor: pointer;
    text-align: left;
    transition: background 0.1s;
  }

  .menu-item:hover {
    background: var(--bg-hover);
  }

  .menu-item.danger:hover {
    background: rgba(211, 47, 47, 0.2);
    color: var(--danger);
  }
</style>
