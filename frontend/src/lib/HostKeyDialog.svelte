<script lang="ts">
  // Every string here is under the security namespace, which a language pack on disk may translate
  // for a new language but may never override for one that ships translated. This is the dialog
  // that stands between a user and a forged server, and a pack able to reword "the host key has
  // changed" into a reassurance would be a way to talk them through accepting it.
  import { createEventDispatcher } from 'svelte';
  import Modal from './Modal.svelte';
  import { t } from '../i18n/messages';

  export let show = false;
  export let host = '';
  export let keyType = '';
  export let fingerprint = '';
  export let keyBase64 = '';
  export let isMismatch = false;

  const dispatch = createEventDispatcher();
</script>

<Modal
  title={isMismatch ? $t('security.hostkey.mismatch.title') : $t('security.hostkey.unknown.title')}
  {show}
  on:close={() => dispatch('cancel')}
>
  {#if isMismatch}
    <div class="hk-warning">
      <strong>{$t('security.hostkey.mismatch.label')}</strong>
      {$t('security.hostkey.mismatch.body', { host })}
    </div>
  {:else}
    <div class="hk-info">
      {$t('security.hostkey.unknown.body', { host })}
    </div>
  {/if}

  <div class="hk-details">
    <div class="hk-row">
      <span class="hk-label">{$t('security.hostkey.field.host')}</span>
      <span class="hk-value">{host}</span>
    </div>
    <div class="hk-row">
      <span class="hk-label">{$t('security.hostkey.field.keyType')}</span>
      <span class="hk-value">{keyType}</span>
    </div>
    <div class="hk-row">
      <span class="hk-label">{$t('security.hostkey.field.fingerprint')}</span>
      <span class="hk-value hk-fp">{fingerprint}</span>
    </div>
  </div>

  <div class="hk-question">
    {#if isMismatch}
      {$t('security.hostkey.mismatch.question')}
    {:else}
      {$t('security.hostkey.unknown.question')}
    {/if}
  </div>

  <div class="hk-actions">
    <button on:click={() => dispatch('accept')}>
      {#if isMismatch}
        {$t('security.hostkey.mismatch.accept')}
      {:else}
        {$t('security.hostkey.unknown.accept')}
      {/if}
    </button>
    <button class="secondary" on:click={() => dispatch('cancel')}>{$t('security.hostkey.cancel')}</button>
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
