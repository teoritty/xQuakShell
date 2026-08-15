<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import Modal from './Modal.svelte';

  // The remote identity a plugin protocol is asking about. Separate from HostKeyDialog on purpose:
  // that one speaks about SSH host keys, this one speaks about whatever identity the protocol
  // presented, and neither should be reworded by a change to the other.
  //
  // No key material is a prop here. The backend kept it, shows this fingerprint of it, and records
  // that same material if the user agrees - so there is nothing this dialog could get wrong about
  // what is being trusted.
  export let show = false;
  export let connectionName = '';
  export let subject = '';
  export let fingerprint = '';
  export let isMismatch = false;

  const dispatch = createEventDispatcher();
</script>

<Modal
  title={isMismatch ? 'Remote Identity Changed' : 'Unknown Remote Identity'}
  {show}
  on:close={() => dispatch('reject')}
>
  {#if isMismatch}
    <div class="pt-warning">
      <strong>WARNING:</strong> The identity presented by <code>{subject}</code> is not the one
      trusted before. This could indicate an interception attempt, or that the server was
      reinstalled or reissued its certificate. Only proceed if you know why it changed.
    </div>
  {:else}
    <div class="pt-info">
      This is the first connection to <code>{subject}</code>. Its identity has not been seen before
      and cannot be verified automatically.
    </div>
  {/if}

  <div class="pt-details">
    {#if connectionName}
      <div class="pt-row">
        <span class="pt-label">Connection:</span>
        <span class="pt-value">{connectionName}</span>
      </div>
    {/if}
    <div class="pt-row">
      <span class="pt-label">Address:</span>
      <span class="pt-value">{subject}</span>
    </div>
    <div class="pt-row">
      <span class="pt-label">Fingerprint:</span>
      <span class="pt-value pt-fp">{fingerprint}</span>
    </div>
  </div>

  <div class="pt-question">
    {#if isMismatch}
      Do you want to <strong>replace</strong> the trusted identity for this address with the one
      shown above?
    {:else}
      Do you want to trust this identity and continue connecting?
    {/if}
  </div>

  <div class="pt-actions">
    <button on:click={() => dispatch('trust')}>
      {#if isMismatch}
        Replace Trusted Identity
      {:else}
        Trust and Connect
      {/if}
    </button>
    <button class="secondary" on:click={() => dispatch('reject')}>Cancel</button>
  </div>
</Modal>

<style>
  .pt-warning {
    padding: 10px 14px;
    background: rgba(255, 152, 0, 0.15);
    border: 1px solid var(--warning);
    border-radius: 4px;
    color: var(--warning);
    font-size: 12px;
    margin-bottom: 12px;
    line-height: 1.5;
  }

  .pt-info {
    padding: 10px 14px;
    background: rgba(0, 120, 212, 0.1);
    border: 1px solid var(--accent);
    border-radius: 4px;
    color: var(--text-primary);
    font-size: 13px;
    margin-bottom: 12px;
    line-height: 1.5;
  }

  .pt-details {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-bottom: 12px;
    padding: 10px;
    background: var(--bg-tertiary);
    border-radius: 4px;
  }

  .pt-row {
    display: flex;
    gap: 8px;
    font-size: 12px;
  }

  .pt-label {
    color: var(--text-secondary);
    min-width: 80px;
    flex-shrink: 0;
  }

  .pt-value {
    color: var(--text-bright);
    word-break: break-all;
  }

  .pt-fp {
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

  .pt-question {
    font-size: 13px;
    color: var(--text-primary);
    margin-bottom: 16px;
    line-height: 1.5;
  }

  .pt-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }

  .pt-actions button {
    padding: 6px 16px;
    font-size: 13px;
  }
</style>
