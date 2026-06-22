<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { X } from 'lucide-svelte';
  export let title: string = '';
  export let show: boolean = false;
  export let contentClass: string = '';
  const dispatch = createEventDispatcher();

  function close() {
    dispatch('close');
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') close();
  }
</script>

{#if show}
  <div class="modal-backdrop" on:click={close} on:keydown={handleKeydown}>
    <div class="modal-content {contentClass}" on:click|stopPropagation on:keydown|stopPropagation>
      <div class="modal-header" class:has-header-center={$$slots['header-center']}>
        <span class="modal-title">{title}</span>
        {#if $$slots['header-center']}
          <div class="modal-header-center">
            <slot name="header-center" />
          </div>
        {/if}
        <button class="modal-close" on:click={close}><X size={14} /></button>
      </div>
      <div class="modal-body">
        <slot />
      </div>
    </div>
  </div>
{/if}

<style>
  .modal-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }

  .modal-content {
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 6px;
    min-width: 360px;
    max-width: 560px;
    max-height: 80vh;
    display: flex;
    flex-direction: column;
    box-shadow: 0 8px 32px rgba(0,0,0,0.5);
  }

  .modal-content.scripts-modal {
    width: 60vw;
    max-width: 60vw;
  }

  .modal-content.settings-modal {
    width: 620px;
    max-width: 620px;
  }

  .modal-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 16px;
    border-bottom: 1px solid var(--border-color);
    gap: 8px;
  }

  .modal-header.has-header-center {
    display: grid;
    grid-template-columns: 1fr minmax(200px, 350px) 1fr;
    align-items: center;
  }

  .modal-header.has-header-center .modal-title {
    justify-self: start;
  }

  .modal-header-center {
    justify-self: center;
    width: 100%;
  }

  .modal-header.has-header-center .modal-close {
    justify-self: end;
  }

  .modal-title {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-bright);
  }

  .modal-close {
    background: transparent;
    color: var(--text-secondary);
    padding: 2px 6px;
    font-size: 14px;
  }

  .modal-close:hover {
    color: var(--text-bright);
    background: var(--bg-hover);
  }

  .modal-body {
    padding: 16px;
    overflow-y: auto;
  }
</style>
