<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { Trash2, Star } from 'lucide-svelte';

  export let x = 0;
  export let y = 0;
  export let show = false;
  export let isConnection = false;
  export let isFavorite = false;

  const dispatch = createEventDispatcher<{
    delete: void;
    toggleFavorite: void;
  }>();
</script>

{#if show}
  <div
    class="context-menu"
    style="left: {x}px; top: {y}px"
    role="menu"
    on:click|stopPropagation
  >
    {#if isConnection}
      <button class="menu-item" on:click={() => dispatch('toggleFavorite')} role="menuitem">
        <span class="star-icon" class:filled={isFavorite}><Star size={12} /></span>
        <span>{isFavorite ? 'Remove from favorites' : 'Add to favorites'}</span>
      </button>
    {/if}
    <button class="menu-item danger" on:click={() => dispatch('delete')} role="menuitem">
      <Trash2 size={12} />
      <span>Delete</span>
    </button>
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
</style>
