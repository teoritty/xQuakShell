<script lang="ts">
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
    if (!comment.trim()) return 'Give the key a name so you can tell it apart from the others.';
    if (mode === 'import' && !pemBase64) return 'Choose a private key file to import.';
    if (mode === 'generate' && passphrase !== confirmPassphrase) return 'The two passphrases do not match.';
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

<Modal title="Add a key" {show} on:close={() => { reset(); dispatch('close'); }}>
  <div class="tabs" role="tablist">
    <button role="tab" aria-selected={mode === 'generate'} class:active={mode === 'generate'} on:click={() => (mode = 'generate')}>
      Generate
    </button>
    <button role="tab" aria-selected={mode === 'import'} class:active={mode === 'import'} on:click={() => (mode = 'import')}>
      Import a file
    </button>
  </div>

  <div class="body">
    <label for="key-name">Name</label>
    <input id="key-name" bind:value={comment} placeholder="prod-servers" />

    {#if mode === 'generate'}
      <label for="key-algorithm">Algorithm</label>
      <select id="key-algorithm" bind:value={algorithm}>
        <option value="ed25519">Ed25519 — recommended</option>
        <option value="rsa">RSA</option>
        <option value="ecdsa">ECDSA</option>
      </select>

      {#if algorithm === 'rsa'}
        <label for="key-bits">Size</label>
        <select id="key-bits" bind:value={bits}>
          <option value={4096}>4096 bits</option>
          <option value={3072}>3072 bits</option>
          <option value={2048}>2048 bits</option>
        </select>
      {:else if algorithm === 'ecdsa'}
        <label for="key-curve">Curve</label>
        <select id="key-curve" bind:value={bits}>
          <option value={256}>nistp256</option>
          <option value={384}>nistp384</option>
          <option value={521}>nistp521</option>
        </select>
      {/if}

      <label for="key-passphrase">Passphrase <span class="hint">optional</span></label>
      <input id="key-passphrase" type="password" bind:value={passphrase} autocomplete="new-password" />
      <p class="explain">
        Leave this empty and the vault protects the key: unlocking the vault is enough to use it.
        Set one and the key stays shut even while the vault is open — nothing stored can open it
        without you.
      </p>

      <label for="key-passphrase-confirm">Repeat passphrase</label>
      <input id="key-passphrase-confirm" type="password" bind:value={confirmPassphrase} autocomplete="new-password" />
    {:else}
      <label for="key-file">Private key file</label>
      <input id="key-file" type="file" on:change={onFile} />
      {#if fileName}<p class="explain">Selected: {fileName}</p>{/if}

      <label for="key-import-passphrase">Its passphrase <span class="hint">if it has one</span></label>
      <input id="key-import-passphrase" type="password" bind:value={importPassphrase} autocomplete="off" />
      <p class="explain">
        The file is re-encrypted into the vault. A key with no passphrase of its own is protected by
        the vault rather than stored as-is.
      </p>
    {/if}

    <KeyPolicyFields bind:options showNonExportable={mode === 'generate'} hasPassphrase={mode === 'generate' ? !!passphrase : !!importPassphrase} />

    {#if error}<p class="error">{error}</p>{/if}
  </div>

  <div class="dialog-actions">
    <button on:click={() => { reset(); dispatch('close'); }}>Cancel</button>
    <button class="primary" on:click={submit} disabled={busy}>{mode === 'generate' ? 'Generate' : 'Import'}</button>
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
    color: var(--text-secondary, #888);
    cursor: pointer;
  }

  .tabs button.active {
    color: var(--text-primary, #ddd);
    border-bottom-color: var(--accent, #4a9eff);
  }

  .body {
    display: flex;
    flex-direction: column;
    gap: 5px;
  }

  label {
    font-size: 12px;
    color: var(--text-secondary, #888);
    margin-top: 6px;
  }

  .hint {
    opacity: 0.7;
  }

  input,
  select {
    padding: 6px 8px;
    font-size: 13px;
    border-radius: 4px;
    border: 1px solid var(--border, rgba(255, 255, 255, 0.14));
    background: var(--bg-input, rgba(0, 0, 0, 0.25));
    color: var(--text-primary, #ddd);
  }

  .explain {
    margin: 2px 0 0;
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
