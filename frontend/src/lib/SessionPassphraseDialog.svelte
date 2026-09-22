<script lang="ts">
  // Asks for the passphrase of an encrypted key while a connection waits on it. Not to be confused
  // with keys/KeyPassphraseDialog.svelte, which changes a key's passphrase in the Key Manager; this
  // one only opens a key, once, for the connection that asked.
  //
  // The strings live under the security namespace for the same reason the host key dialog's do: a
  // language pack must not be able to reword a prompt that collects a secret.
  import { createEventDispatcher, onMount } from 'svelte';
  import Modal from './Modal.svelte';
  import { t } from '../i18n/messages';

  export let show = false;
  export let label = '';

  const dispatch = createEventDispatcher<{ submit: { passphrase: string }; cancel: void }>();

  let passphrase = '';
  let input: HTMLInputElement;

  onMount(() => input?.focus());

  // The field is cleared as soon as the value is handed on, so the typed secret does not sit in the
  // component's state for as long as the dialog happens to stay mounted.
  function submit() {
    if (!passphrase) return;
    const value = passphrase;
    passphrase = '';
    dispatch('submit', { passphrase: value });
  }

  function cancel() {
    passphrase = '';
    dispatch('cancel');
  }
</script>

<Modal title={$t('security.passphrase.title')} {show} on:close={cancel}>
  <form on:submit|preventDefault={submit}>
    <p class="pp-body">{$t('security.passphrase.body', { label })}</p>

    <label for="session-passphrase">{$t('security.passphrase.field')}</label>
    <input
      id="session-passphrase"
      type="password"
      autocomplete="off"
      bind:this={input}
      bind:value={passphrase}
    />

    <div class="pp-actions">
      <button type="button" class="secondary" on:click={cancel}>{$t('security.passphrase.cancel')}</button>
      <button type="submit" class="primary" disabled={!passphrase}>{$t('security.passphrase.submit')}</button>
    </div>
  </form>
</Modal>

<style>
  .pp-body {
    margin: 0 0 12px;
    font-size: 13px;
    line-height: 1.5;
    color: var(--text-primary);
  }

  label {
    display: block;
    margin-bottom: 4px;
    font-size: 12px;
    color: var(--text-secondary);
  }

  input {
    width: 100%;
    box-sizing: border-box;
  }

  .pp-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    margin-top: 16px;
  }
</style>
