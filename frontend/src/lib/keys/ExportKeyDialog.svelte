<script lang="ts">
  import { t } from '../../i18n/messages';
  import { createEventDispatcher } from 'svelte';
  import Modal from '../Modal.svelte';
  import type { StoredKey } from '../../api/keys';

  export let show = false;
  export let target: StoredKey | null = null;
  export let error = '';

  const dispatch = createEventDispatcher();

  let masterPassword = '';
  let passphrase = '';
  let exportPassphrase = '';
  let confirmExport = '';
  let busy = false;

  function reset() {
    masterPassword = '';
    passphrase = '';
    exportPassphrase = '';
    confirmExport = '';
    error = '';
  }

  function close() {
    // Every field here holds a secret. Clearing on close keeps them out of the DOM for the rest
    // of the session rather than until the component happens to be destroyed.
    reset();
    dispatch('close');
  }

  async function submit() {
    if (!masterPassword) {
      error = $t('keys.export.needMaster');
      return;
    }
    if (exportPassphrase !== confirmExport) {
      error = $t('keys.export.mismatch');
      return;
    }
    busy = true;
    error = '';
    try {
      dispatch('submit', { masterPassword, passphrase, exportPassphrase });
      reset();
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={$t('keys.export.title', { name: target?.comment || $t('keys.fallbackName') })} {show} on:close={close}>
  <p class="warn">
    The private key will be written to a file outside the vault. Anything that can read that file
    can log in as you. Delete it once you have moved it where it needs to go.
  </p>

  <label for="export-master">{$t('vault.field.master')}</label>
  <input id="export-master" type="password" bind:value={masterPassword} autocomplete="off" />

  {#if target?.policy === 'passphrase'}
    <label for="export-key-passphrase">{$t('keys.export.keyPassphrase')}</label>
    <input id="export-key-passphrase" type="password" bind:value={passphrase} autocomplete="off" />
  {/if}

  <label for="export-new-passphrase">
    {$t('keys.export.protectWith')} <span class="hint">{$t('keys.export.recommended')}</span>
  </label>
  <input id="export-new-passphrase" type="password" bind:value={exportPassphrase} autocomplete="new-password" />

  <label for="export-new-confirm">{$t('keys.export.repeat')}</label>
  <input id="export-new-confirm" type="password" bind:value={confirmExport} autocomplete="new-password" />
  <p class="explain">{$t('security.keys.export.unprotected')}</p>

  {#if error}<p class="error">{error}</p>{/if}

  <div class="dialog-actions">
    <button on:click={close}>{$t('common.cancel')}</button>
    <button class="primary" on:click={submit} disabled={busy}>{$t('keys.export.submit')}</button>
  </div>
</Modal>

<style>
  .dialog-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 14px;
  }



  .warn {
    margin: 0 0 10px;
    padding: 8px 10px;
    border-radius: 4px;
    background: rgba(196, 144, 64, 0.16);
    color: var(--warning);
    font-size: 12px;
    line-height: 1.45;
  }

  label {
    display: block;
    margin-top: 8px;
    font-size: 12px;
    color: var(--text-secondary);
  }

  .hint {
    opacity: 0.7;
  }


  .explain {
    margin: 4px 0 0;
    font-size: 11px;
    color: var(--text-secondary);
  }

  .error {
    margin: 8px 0 0;
    font-size: 12px;
    color: var(--danger);
  }

</style>
