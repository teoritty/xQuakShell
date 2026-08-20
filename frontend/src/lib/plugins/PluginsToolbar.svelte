<script lang="ts">
  // Search box plus the overflow menu, above the rail.
  //
  // Local installs live in this menu rather than beside Browse because a folder on this machine is
  // not a source: putting it in the Sources list would blur the one distinction that section
  // exists to draw, and the trust warning it needs is different in kind from an untrusted
  // repository's.
  import { createEventDispatcher } from 'svelte';
  import { MoreHorizontal, Search } from 'lucide-svelte';

  export let query = '';
  /**
   * False on a section with nothing to search, where the box stays in place and goes inert.
   *
   * It used to be removed instead, which made the toolbar reflow on every rail click and read as a
   * layout bug. A disabled control that says why is the honest version of the same statement: the
   * box is still visibly there, and the one thing it must not do is silently accept typing and
   * change nothing.
   */
  export let searchable = true;

  const dispatch = createEventDispatcher();

  let menuOpen = false;

  function pick(event: string) {
    menuOpen = false;
    dispatch(event);
  }
</script>

<div class="plugins-toolbar">
  <div class="search-box" class:inert={!searchable}>
    <Search size={13} />
    <input
      type="text"
      placeholder={searchable ? 'Search by name, description or source' : 'Nothing to search here'}
      disabled={!searchable}
      bind:value={query}
      on:input={() => dispatch('search', { query })}
    />
  </div>
  <div class="menu-wrap">
    <button class="ghost icon-btn" title="More actions" on:click={() => (menuOpen = !menuOpen)}>
      <MoreHorizontal size={15} />
    </button>
    {#if menuOpen}
      <!-- svelte-ignore a11y_no_static_element_interactions -->
      <div class="menu" on:mouseleave={() => (menuOpen = false)}>
        <button class="menu-item" on:click={() => pick('installFolder')}>Install from folder…</button>
        <button class="menu-item" on:click={() => pick('installBundle')}>Install from bundle…</button>
        <div class="menu-sep"></div>
        <button class="menu-item" on:click={() => pick('refreshAll')}>Refresh all sources</button>
      </div>
    {/if}
  </div>
</div>

<style>
  .plugins-toolbar {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 10px;
  }

  .search-box {
    display: flex;
    align-items: center;
    gap: 6px;
    flex: 1;
    padding: 4px 8px;
    border: 1px solid var(--border-color);
    border-radius: 5px;
    background: var(--bg-secondary);
    color: var(--text-secondary);
  }

  .search-box.inert {
    opacity: 0.5;
  }

  .search-box input {
    flex: 1;
    border: none;
    background: transparent;
    outline: none;
    font-size: 12px;
    color: var(--text-primary);
  }

  .search-box input:disabled {
    cursor: default;
  }

  .menu-wrap {
    position: relative;
  }

  .menu {
    position: absolute;
    right: 0;
    top: calc(100% + 4px);
    z-index: 10;
    min-width: 190px;
    padding: 4px;
    border: 1px solid var(--border-color);
    border-radius: 6px;
    background: var(--bg-tertiary);
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.35);
  }

  .menu-item {
    display: block;
    width: 100%;
    padding: 6px 9px;
    border: none;
    border-radius: 4px;
    background: transparent;
    color: inherit;
    font-size: 12px;
    text-align: left;
    cursor: pointer;
  }

  .menu-item:hover {
    background: var(--bg-hover);
  }

  .menu-sep {
    height: 1px;
    margin: 4px 2px;
    background: var(--border-color);
  }

  .icon-btn {
    display: inline-flex;
    align-items: center;
    padding: 4px;
  }
</style>
