<script lang="ts">
  import { t } from '../../i18n/messages';
  import { createEventDispatcher } from 'svelte';
  import Modal from '../Modal.svelte';
  import KeyPolicyFields from './KeyPolicyFields.svelte';
  import { defaultKeyOptions, type KeyOptions } from '../../api/keys';

  export let show = false;

  const dispatch = createEventDispatcher();

  let mode: 'generate' | 'import' = 'generate';
  let algorithm = 'ed25519';
  let bits = 4096;
  let comment = '';
  let passphrase = '';
  let confirmPassphrase = '';
  let importPassphrase = '';
  let pemBase64 = '';
  let fileName = '';
  let options: KeyOptions = { ...defaultKeyOptions };
  let error = '';
  let busy = false;

  $: if (show) {
    // Nothing is carried over between openings: a passphrase left in a field from the last time
    // this dialog was open is a secret sitting in the DOM for no reason.
    error = '';
  }

  function reset() {
    algorithm = 'ed25519';
    bits = 4096;
    comment = '';
    passphrase = '';
    confirmPassphrase = '';
    importPassphrase = '';
    pemBase64 = '';
    fileName = '';
    options = { ...defaultKeyOptions };
  }

  async function onFile(event: Event) {
    const input = event.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    fileName = file.name;
    if (!comment) comment = file.name;
    const buffer = await file.arrayBuffer();
    pemBase64 = base64Of(new Uint8Array(buffer));
  }

  // btoa needs a binary string; a chunked loop avoids blowing the argument limit on a large key.
  function base64Of(bytes: Uint8Array): string {
    let binary = '';
    for (let i = 0; i < bytes.length; i += 1) binary += String.fromCharCode(bytes[i]);
    return btoa(binary);
  }

  function validate(): string {
    if (!comment.trim()) return $t('keys.new.needName');
    if (mode === 'import' && !pemBase64) return $t('keys.new.needFile');
    if (mode === 'generate' && passphrase !== confirmPassphrase) return $t('keys.passphrase.mismatch');
    return '';
  }

  async function submit() {
    error = validate();
    if (error) return;
    busy = true;
    try {
      dispatch('submit', {
        mode,
        algorithm,
        bits: algorithm === 'ed25519' ? 0 : bits,
        comment: comment.trim(),
        passphrase: mode === 'generate' ? passphrase : importPassphrase,
        pemBase64,
        options,
      });
      reset();
    } finally {
      busy = false;
    }
  }
</script>

<Modal title={$t('keys.new.title')} {show} on:close={() => { reset(); dispatch('close'); }}>
  <div class="tabs" role="tablist">
    <button role="tab" aria-selected={mode === 'generate'} class:active={mode === 'generate'} on:click={() => (mode = 'generate')}>
      {$t('keys.new.generate')}
    </button>
    <button role="tab" aria-selected={mode === 'import'} class:active={mode === 'import'} on:click={() => (mode = 'import')}>
      {$t('keys.new.import')}
    </button>
  </div>

  <div class="body">
    <label for="key-name">{$t('keys.field.name')}</label>
    <input id="key-name" bind:value={comment} placeholder={$t('keys.new.namePlaceholder')} />

    {#if mode === 'generate'}
      <label for="key-algorithm">{$t('keys.new.algorithm')}</label>
      <select id="key-algorithm" bind:value={algorithm}>
        <option value="ed25519">{$t('keys.new.ed25519')}</option>
        <option value="rsa">RSA</option>
        <option value="ecdsa">ECDSA</option>
      </select>

      {#if algorithm === 'rsa'}
        <label for="key-bits">{$t('keys.new.size')}</label>
        <select id="key-bits" bind:value={bits}>
          <option value={4096}>{$t('keys.new.bits', { bits: 4096 })}</option>
          <option value={3072}>{$t('keys.new.bits', { bits: 3072 })}</option>
          <option value={2048}>{$t('keys.new.bits', { bits: 2048 })}</option>
        </select>
      {:else if algorithm === 'ecdsa'}
        <label for="key-curve">{$t('keys.new.curve')}</label>
        <select id="key-curve" bind:value={bits}>
          <option value={256}>nistp256</option>
          <option value={384}>nistp384</option>
          <option value={521}>nistp521</option>
        </select>
      {/if}

      <label for="key-passphrase">
        {$t('keys.field.passphrase')} <span class="hint">{$t('keys.new.optional')}</span>
      </label>
      <input id="key-passphrase" type="password" bind:value={passphrase} autocomplete="new-password" />
      <p class="explain">{$t('security.keys.new.passphraseExplain')}</p>

      <label for="key-passphrase-confirm">{$t('keys.new.repeatPassphrase')}</label>
      <input id="key-passphrase-confirm" type="password" bind:value={confirmPassphrase} autocomplete="new-password" />
    {:else}
      <label for="key-file">{$t('keys.new.file')}</label>
      <input id="key-file" type="file" on:change={onFile} />
      {#if fileName}<p class="explain">{$t('keys.new.selected', { name: fileName })}</p>{/if}

      <label for="key-import-passphrase">
        {$t('keys.new.itsPassphrase')} <span class="hint">{$t('keys.new.ifAny')}</span>
      </label>
      <input id="key-import-passphrase" type="password" bind:value={importPassphrase} autocomplete="off" />
      <p class="explain">{$t('security.keys.new.importExplain')}</p>
    {/if}

    <KeyPolicyFields bind:options showNonExportable={mode === 'generate'} hasPassphrase={mode === 'generate' ? !!passphrase : !!importPassphrase} />

    {#if error}<p class="error">{error}</p>{/if}
  </div>

  <div class="dialog-actions">
    <button on:click={() => { reset(); dispatch('close'); }}>{$t('common.cancel')}</button>
    <button class="primary" on:click={submit} disabled={busy}>{mode === 'generate' ? $t('keys.new.generate') : $t('keys.new.importSubmit')}</button>
  </div>
</Modal>

<style>
  .dialog-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 14px;
  }



  .tabs {
    display: flex;
    gap: 4px;
    margin-bottom: 10px;
  }

  .tabs button {
    flex: 1;
    padding: 6px 10px;
    font-size: 12px;
    background: transparent;
    border: none;
    border-bottom: 2px solid transparent;
    color: var(--text-secondary);
    cursor: pointer;
  }

  .tabs button.active {
    color: var(--text-primary);
    border-bottom-color: var(--accent);
  }

  .body {
    display: flex;
    flex-direction: column;
    gap: 5px;
  }

  label {
    font-size: 12px;
    color: var(--text-secondary);
    margin-top: 6px;
  }

  .hint {
    opacity: 0.7;
  }


  .explain {
    margin: 2px 0 0;
    font-size: 11px;
    line-height: 1.45;
    color: var(--text-secondary);
  }

  .error {
    margin: 8px 0 0;
    font-size: 12px;
    color: var(--danger);
  }

</style>
