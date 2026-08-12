<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import Modal from '../Modal.svelte';
  import type { StoredKey } from '../../api/keys';

  export let show = false;
  export let target: StoredKey | null = null;
  export let error = '';

  const dispatch = createEventDispatcher();

  let oldPassphrase = '';
  let newPassphrase = '';
  let confirmPassphrase = '';

  function close() {
    oldPassphrase = '';
    newPassphrase = '';
    confirmPassphrase = '';
    dispatch('close');
  }

  function submit() {
    if (newPassphrase !== confirmPassphrase) {
      error = 'The two passphrases do not match.';
      return;
    }
    dispatch('submit', { oldPassphrase, newPassphrase });
  }
</script>

<Modal title="Passphrase for {target?.comment || 'key'}" {show} on:close={close}>
  {#if target?.migrationPending}
    <p class="note">
      Entering the current passphrase here also finishes this key's upgrade, after which it can be
      exported and published again.
    </p>
  {/if}

  {#if target?.policy === 'passphrase'}
    <label for="pp-old">Current passphrase</label>
    <input id="pp-old" type="password" bind:value={oldPassphrase} autocomplete="off" />
  {/if}

  <label for="pp-new">New passphrase <span class="hint">leave empty to let the vault protect it</span></label>
  <input id="pp-new" type="password" bind:value={newPassphrase} autocomplete="new-password" />

  <label for="pp-confirm">Repeat</label>
  <input id="pp-confirm" type="password" bind:value={confirmPassphrase} autocomplete="new-password" />

  <p class="explain">
    With no passphrase the key opens whenever the vault is unlocked. With one, it stays shut until
    you type it, even while the vault is open.
  </p>

  {#if error}<p class="error">{error}</p>{/if}

  <div class="dialog-actions">
    <button on:click={close}>Cancel</button>
    <button class="primary" on:click={submit}>Save</button>
  </div>
</Modal>

<style>
  .dialog-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 14px;
  }

  .dialog-actions button {
    padding: 6px 14px;
    font-size: 12px;
    border-radius: 4px;
    border: 1px solid var(--border, rgba(255, 255, 255, 0.14));
    background: var(--bg-button, rgba(255, 255, 255, 0.06));
    color: var(--text-primary, #ddd);
    cursor: pointer;
  }

  .dialog-actions button:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

  label {
    display: block;
    margin-top: 8px;
    font-size: 12px;
    color: var(--text-secondary, #888);
  }

  .hint {
    opacity: 0.7;
  }

  input {
    width: 100%;
    padding: 6px 8px;
    font-size: 13px;
    border-radius: 4px;
    border: 1px solid var(--border, rgba(255, 255, 255, 0.14));
    background: var(--bg-input, rgba(0, 0, 0, 0.25));
    color: var(--text-primary, #ddd);
  }

  .note {
    margin: 0 0 10px;
    padding: 8px 10px;
    border-radius: 4px;
    background: var(--warn-bg, rgba(255, 176, 0, 0.14));
    color: var(--warn-fg, #ffb000);
    font-size: 12px;
    line-height: 1.45;
  }

  .explain {
    margin: 8px 0 0;
    font-size: 11px;
    line-height: 1.45;
    color: var(--text-secondary, #888);
  }

  .error {
    margin: 8px 0 0;
    font-size: 12px;
    color: var(--danger, #ff6b6b);
  }

  button.primary {
    background: var(--accent, #4a9eff);
    color: #fff;
    border-color: transparent;
  }
</style>
