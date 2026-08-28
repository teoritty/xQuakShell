<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import Modal from './Modal.svelte';
  import { t } from '../i18n/messages';

  // Every string here is under the security namespace, so a language pack on disk can translate it
  // for a new language but never reword it for one that ships translated.
  //
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
  title={isMismatch ? $t('security.peertrust.mismatch.title') : $t('security.peertrust.unknown.title')}
  {show}
  on:close={() => dispatch('reject')}
>
  {#if isMismatch}
    <div class="pt-warning">
      <strong>{$t('security.peertrust.mismatch.label')}</strong>
      {$t('security.peertrust.mismatch.body', { subject })}
    </div>
  {:else}
    <div class="pt-info">
      {$t('security.peertrust.unknown.body', { subject })}
    </div>
  {/if}

  <div class="pt-details">
    {#if connectionName}
      <div class="pt-row">
        <span class="pt-label">{$t('security.peertrust.field.connection')}</span>
        <span class="pt-value">{connectionName}</span>
      </div>
    {/if}
    <div class="pt-row">
      <span class="pt-label">{$t('security.peertrust.field.address')}</span>
      <span class="pt-value">{subject}</span>
    </div>
    <div class="pt-row">
      <span class="pt-label">{$t('security.peertrust.field.fingerprint')}</span>
      <span class="pt-value pt-fp">{fingerprint}</span>
    </div>
  </div>

  <div class="pt-question">
    {#if isMismatch}
      {$t('security.peertrust.mismatch.question')}
    {:else}
      {$t('security.peertrust.unknown.question')}
    {/if}
  </div>

  <div class="pt-actions">
    <button on:click={() => dispatch('trust')}>
      {#if isMismatch}
        {$t('security.peertrust.mismatch.accept')}
      {:else}
        {$t('security.peertrust.unknown.accept')}
      {/if}
    </button>
    <button class="secondary" on:click={() => dispatch('reject')}>{$t('security.peertrust.cancel')}</button>
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
