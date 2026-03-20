<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import Modal from './Modal.svelte';

  export let show = false;
  export let host = '';
  export let keyType = '';
  export let fingerprint = '';
  export let keyBase64 = '';
  export let isMismatch = false;

  const dispatch = createEventDispatcher();
</script>

<Modal
  title={isMismatch ? 'Host Key Mismatch' : 'Unknown Host Key'}
  {show}
  on:close={() => dispatch('cancel')}
>
  {#if isMismatch}
    <div class="hk-warning">
      <strong>WARNING:</strong> The host key for <code>{host}</code> has changed!
      This could indicate a man-in-the-middle attack or that the server was reinstalled.
      Only proceed if you are sure this change is expected.
    </div>
  {:else}
    <div class="hk-info">
      The authenticity of host <code>{host}</code> cannot be established.
    </div>
  {/if}

  <div class="hk-details">
    <div class="hk-row">
      <span class="hk-label">Host:</span>
      <span class="hk-value">{host}</span>
    </div>
    <div class="hk-row">
      <span class="hk-label">Key type:</span>
      <span class="hk-value">{keyType}</span>
    </div>
    <div class="hk-row">
      <span class="hk-label">Fingerprint:</span>
      <span class="hk-value hk-fp">{fingerprint}</span>
    </div>
  </div>

  <div class="hk-question">
    {#if isMismatch}
      Do you want to <strong>replace</strong> the existing key with this new one?
    {:else}
      Are you sure you want to continue connecting and add this key to known hosts?
    {/if}
  </div>

  <div class="hk-actions">
    <button on:click={() => dispatch('accept')}>
      {#if isMismatch}
        Replace Key
      {:else}
        Add to Known Hosts
      {/if}
    </button>
    <button class="secondary" on:click={() => dispatch('cancel')}>Cancel</button>
  </div>
</Modal>

<style>
  .hk-warning {
    padding: 10px 14px;
    background: rgba(255, 152, 0, 0.15);
    border: 1px solid var(--warning);
    border-radius: 4px;
    color: var(--warning);
    font-size: 12px;
    margin-bottom: 12px;
    line-height: 1.5;
  }

  .hk-info {
    padding: 10px 14px;
    background: rgba(0, 120, 212, 0.1);
    border: 1px solid var(--accent);
    border-radius: 4px;
    color: var(--text-primary);
    font-size: 13px;
    margin-bottom: 12px;
  }

  .hk-details {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-bottom: 12px;
    padding: 10px;
    background: var(--bg-tertiary);
    border-radius: 4px;
  }

  .hk-row {
    display: flex;
    gap: 8px;
    font-size: 12px;
  }

  .hk-label {
    color: var(--text-secondary);
    min-width: 80px;
    flex-shrink: 0;
  }

  .hk-value {
    color: var(--text-bright);
    word-break: break-all;
  }

  .hk-fp {
    font-family: var(--font-mono);
    font-size: 11px;
  }

  code {
    background: var(--bg-input);
    padding: 1px 4px;
    border-radius: 2px;
    font-family: var(--font-mono);
    font-size: 12px;
  }

  .hk-error {
    padding: 8px 12px;
    background: rgba(211, 47, 47, 0.15);
    border: 1px solid var(--danger);
    border-radius: 4px;
    color: var(--danger);
    font-size: 12px;
    margin-bottom: 12px;
  }

  .hk-question {
    font-size: 13px;
    color: var(--text-primary);
    margin-bottom: 16px;
    line-height: 1.5;
  }

  .hk-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }

  .hk-actions button {
    padding: 6px 16px;
    font-size: 13px;
  }
</style>
