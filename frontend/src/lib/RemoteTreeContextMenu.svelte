<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { FolderPlus, MonitorDot, Pencil, Star, Trash2 } from 'lucide-svelte';
  import type { DiscoveryMenu, DiscoveryMenuItem } from './remoteTree/discoveryActions';

  export let x = 0;
  export let y = 0;
  export let show = false;
  export let isFolder = false;
  export let isConnection = false;
  export let isFavorite = false;
  /**
   * Discovery rows replace the whole static menu rather than adding to it: the
   * core's items (edit, delete, favourite) act on local config and mean nothing
   * for a remote resource a plugin drew. Already computed by
   * discoveryActions.ts — this component only draws it.
   */
  export let discoveryMenu: DiscoveryMenu | null = null;

  const dispatch = createEventDispatcher<{
    newConnection: void;
    newFolder: void;
    edit: void;
    delete: void;
    toggleFavorite: void;
    invokeAction: DiscoveryMenuItem;
  }>();
</script>

{#if show}
  <div class="context-menu" style="left: {x}px; top: {y}px" role="menu" on:click|stopPropagation>
    {#if discoveryMenu}
      {#each discoveryMenu.items as item (item.id)}
        <button
          class="menu-item"
          class:danger={item.danger}
          disabled={item.disabled}
          on:click={() => !item.disabled && dispatch('invokeAction', item)}
          role="menuitem"
        >
          <span>{item.label}</span>
        </button>
      {/each}
      <!-- An empty menu still says why. A menu that simply is not there reads as
           a bug, and "no action applies to all 4 selected items" is an answer. -->
      {#if discoveryMenu.notice}
        <div class="menu-notice">{discoveryMenu.notice}</div>
      {/if}
    {:else}
    {#if isFolder}
      <button class="menu-item" on:click={() => dispatch('newConnection')} role="menuitem">
        <MonitorDot size={12} />
        <span>New connection</span>
      </button>
      <button class="menu-item" on:click={() => dispatch('newFolder')} role="menuitem">
        <FolderPlus size={12} />
        <span>New folder</span>
      </button>
      <button class="menu-item" on:click={() => dispatch('edit')} role="menuitem">
        <Pencil size={12} />
        <span>Edit</span>
      </button>
    {/if}
    {#if isConnection}
      <button class="menu-item" on:click={() => dispatch('edit')} role="menuitem">
        <Pencil size={12} />
        <span>Edit</span>
      </button>
      <button class="menu-item" on:click={() => dispatch('toggleFavorite')} role="menuitem">
        <span class="star-icon" class:filled={isFavorite}><Star size={12} /></span>
        <span>{isFavorite ? 'Remove from favorites' : 'Add to favorites'}</span>
      </button>
    {/if}
    <button class="menu-item danger" on:click={() => dispatch('delete')} role="menuitem">
      <Trash2 size={12} />
      <span>Delete</span>
    </button>
    {/if}
  </div>
{/if}

<style>
  .context-menu {
    position: fixed;
    z-index: 1000;
    min-width: 180px;
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

  .menu-item .star-icon.filled :global(svg) {
    fill: currentColor;
  }

  .menu-item:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .menu-item:disabled:hover {
    background: transparent;
  }

  .menu-notice {
    padding: 6px 12px;
    max-width: 260px;
    font-size: 11px;
    color: var(--text-secondary);
    white-space: normal;
  }
</style>
