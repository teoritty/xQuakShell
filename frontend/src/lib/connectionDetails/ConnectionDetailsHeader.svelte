<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { X } from 'lucide-svelte';
  import type { SaveStatus } from './types';

  export let saveStatus: SaveStatus = 'idle';

  const dispatch = createEventDispatcher<{ close: void }>();
</script>

<div class="panel-header">
  <div class="panel-header-left">
    <span>Connection</span>
    <span class="save-indicator">
      {#if saveStatus === 'saving'}Saving...{:else if saveStatus === 'saved'}Saved{/if}
    </span>
  </div>
  <button
    type="button"
    class="panel-close-btn"
    title="Close"
    on:click={() => dispatch('close')}
  >
    <X size={14} />
  </button>
</div>

<style>
  .panel-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .panel-header-left {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }

  .panel-close-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: none;
    border: none;
    padding: 2px 4px;
    cursor: pointer;
    color: var(--text-secondary);
    border-radius: 2px;
    flex-shrink: 0;
  }

  .panel-close-btn:hover {
    color: var(--danger);
  }

  .save-indicator {
    font-size: 10px;
    color: var(--text-secondary);
    font-style: italic;
  }
</style>
