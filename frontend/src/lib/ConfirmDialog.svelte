<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { AlertTriangle, X } from 'lucide-svelte';

  export let show = false;
  export let title = 'Confirm';
  export let message = 'Are you sure?';
  export let critical = false;
  export let requireCheckbox = false;
  export let checkboxLabel = '';
  export let confirmLabel = 'Delete';
  export let cancelLabel = 'Cancel';

  const dispatch = createEventDispatcher();

  let checked = false;

  $: if (!show) checked = false;

  function confirm() {
    if (requireCheckbox && !checked) return;
    dispatch('confirm');
  }

  function cancel() {
    dispatch('cancel');
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') cancel();
    if (e.key === 'Enter' && (!requireCheckbox || checked)) confirm();
  }
</script>

{#if show}
  <div class="confirm-backdrop" on:click={cancel} on:keydown={handleKeydown}>
    <div class="confirm-dialog" class:critical on:click|stopPropagation on:keydown|stopPropagation>
      <div class="confirm-header">
        <span class="confirm-title">
          {#if critical}<AlertTriangle size={16} />{/if}
          {title}
        </span>
        <button class="confirm-close" on:click={cancel}><X size={14} /></button>
      </div>
      <div class="confirm-body">
        <p class="confirm-message">{message}</p>
        {#if requireCheckbox}
          <label class="confirm-checkbox">
            <input type="checkbox" bind:checked />
            {checkboxLabel}
          </label>
        {/if}
      </div>
      <div class="confirm-footer">
        <button class="secondary" on:click={cancel}>{cancelLabel}</button>
        <button
          class="danger"
          on:click={confirm}
          disabled={requireCheckbox && !checked}
        >
          {confirmLabel}
        </button>
      </div>
    </div>
  </div>
{/if}

<style>
  .confirm-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 2000;
  }

  .confirm-dialog {
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 6px;
    min-width: 340px;
    max-width: 440px;
    box-shadow: 0 8px 32px rgba(0,0,0,0.5);
  }

  .confirm-dialog.critical {
    border-color: var(--danger, #f44747);
  }

  .confirm-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 16px;
    border-bottom: 1px solid var(--border-color);
  }

  .confirm-title {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-bright);
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .critical .confirm-title {
    color: var(--danger, #f44747);
  }

  .confirm-close {
    background: transparent;
    color: var(--text-secondary);
    padding: 2px 6px;
    border: none;
    cursor: pointer;
    border-radius: 2px;
  }
  .confirm-close:hover {
    color: var(--text-bright);
    background: var(--bg-hover);
  }

  .confirm-body {
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .confirm-message {
    font-size: 13px;
    color: var(--text-primary);
    margin: 0;
    line-height: 1.4;
  }

  .confirm-checkbox {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    color: var(--text-primary);
    cursor: pointer;
  }
  .confirm-checkbox input {
    margin: 0;
    width: 14px;
    height: 14px;
  }

  .confirm-footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 12px 16px;
    border-top: 1px solid var(--border-color);
  }
  .confirm-footer button {
    padding: 5px 14px;
    font-size: 12px;
    border: none;
    border-radius: 3px;
    cursor: pointer;
  }
  .confirm-footer .secondary {
    background: var(--bg-tertiary);
    color: var(--text-primary);
  }
  .confirm-footer .secondary:hover {
    background: var(--bg-hover);
  }
  .confirm-footer .danger {
    background: var(--danger, #f44747);
    color: #fff;
  }
  .confirm-footer .danger:hover {
    opacity: 0.9;
  }
  .confirm-footer .danger:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
</style>
