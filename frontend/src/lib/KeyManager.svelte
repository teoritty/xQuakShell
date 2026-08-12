<script lang="ts">
  import Modal from './Modal.svelte';
  import KeyList from './keys/KeyList.svelte';
  import KeyDetails from './keys/KeyDetails.svelte';
  import NewKeyDialog from './keys/NewKeyDialog.svelte';
  import ExportKeyDialog from './keys/ExportKeyDialog.svelte';
  import KeyPassphraseDialog from './keys/KeyPassphraseDialog.svelte';
  import DeployKeyDialog from './keys/DeployKeyDialog.svelte';
  import KeyPolicyFields from './keys/KeyPolicyFields.svelte';
  import { sessions, showError } from '../stores/appState';
  import { defaultKeyOptions, type KeyOptions, type KeyUsage, type StoredKey } from '../api/keys';
  import {
    addKeyFromFile,
    createKey,
    keysLoading,
    loadKeyUsages,
    publishKey,
    refreshKeys,
    removeKey,
    saveKeyName,
    saveKeyPolicy,
    saveKeyToDisk,
    selectedKeyId,
    storedKeys,
    updateKeyPassphrase,
  } from '../actions/keyActions';

  export let show = false;

  let filter = '';
  let usages: KeyUsage[] = [];
  let copied = false;
  let showNew = false;
  let showExport = false;
  let showPassphrase = false;
  let showDeploy = false;
  let showPolicy = false;
  let policyDraft: KeyOptions = { ...defaultKeyOptions };
  let dialogError = '';
  let busy = false;

  $: selected = $storedKeys.find((key) => key.id === $selectedKeyId) || null;
  $: if (show) void refreshKeys();
  $: void loadUsagesFor(selected);

  async function loadUsagesFor(key: StoredKey | null) {
    usages = key ? await loadKeyUsages(key.id) : [];
  }

  async function copyPublicKey() {
    if (!selected?.publicKey) return;
    await navigator.clipboard.writeText(selected.publicKey);
    copied = true;
    setTimeout(() => (copied = false), 1500);
  }

  async function onCreate(event: CustomEvent) {
    const d = event.detail;
    const ok = d.mode === 'generate'
      ? await createKey(d.algorithm, d.bits, d.comment, d.passphrase, d.options)
      : await addKeyFromFile(d.pemBase64, d.passphrase, d.comment, d.options);
    if (ok) showNew = false;
  }

  async function onRename() {
    if (!selected) return;
    const name = prompt('Name for this key', selected.comment);
    if (name && name.trim()) await saveKeyName(selected.id, name.trim());
  }

  function openPolicy() {
    if (!selected) return;
    policyDraft = {
      cachePolicy: selected.cachePolicy || 'until-lock',
      cacheTtlSeconds: selected.cacheTtlSeconds || defaultKeyOptions.cacheTtlSeconds,
      allowPlugins: selected.allowPlugins,
      nonExportable: selected.nonExportable,
    };
    showPolicy = true;
  }

  async function savePolicy() {
    if (!selected) return;
    await saveKeyPolicy(selected.id, policyDraft);
    showPolicy = false;
  }

  async function onDelete() {
    if (!selected) return;
    const warning = usages.length > 0
      ? `"${selected.comment}" is used by ${usages.length} connection(s) and cannot be deleted until they stop using it.`
      : `Delete "${selected.comment}"? The private key is destroyed and cannot be recovered.`;
    if (usages.length > 0) {
      showError(warning);
      return;
    }
    if (!confirm(warning)) return;
    try {
      await removeKey(selected.id);
    } catch (e) {
      showError(e instanceof Error ? e.message : String(e));
    }
  }

  async function onPassphrase(event: CustomEvent) {
    if (!selected) return;
    dialogError = '';
    try {
      await updateKeyPassphrase(selected.id, event.detail.oldPassphrase, event.detail.newPassphrase);
      showPassphrase = false;
    } catch (e) {
      dialogError = e instanceof Error ? e.message : String(e);
    }
  }

  // The exported key is handed over as a download rather than shown on screen: a private key
  // rendered into the DOM would sit in the page, and in any screenshot of it, for as long as the
  // window stayed open.
  async function onExport(event: CustomEvent) {
    if (!selected) return;
    dialogError = '';
    try {
      const base64 = await saveKeyToDisk(
        selected.id,
        event.detail.masterPassword,
        event.detail.passphrase,
        event.detail.exportPassphrase,
      );
      downloadKey(base64, `${selected.comment || selected.id}.key`);
      showExport = false;
    } catch (e) {
      dialogError = e instanceof Error ? e.message : String(e);
    }
  }

  function downloadKey(base64: string, fileName: string) {
    const bytes = Uint8Array.from(atob(base64), (c) => c.charCodeAt(0));
    const url = URL.createObjectURL(new Blob([bytes], { type: 'application/octet-stream' }));
    const link = document.createElement('a');
    link.href = url;
    link.download = fileName;
    link.click();
    URL.revokeObjectURL(url);
  }

  async function onDeploy(event: CustomEvent) {
    if (!selected) return;
    busy = true;
    dialogError = '';
    try {
      const result = await publishKey(event.detail, selected.id);
      if (result) {
        showDeploy = false;
        showError(result.alreadyPresent ? 'That key was already authorised on this server.' : `Key added to ${result.path}.`);
      }
    } catch (e) {
      dialogError = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }
</script>

<Modal title="SSH Keys" {show} contentClass="key-manager" on:close={() => (show = false)}>
  <div class="layout">
    <aside>
      <input class="search" placeholder="Search by name or fingerprint" bind:value={filter} />
      <KeyList keys={$storedKeys} bind:selectedId={$selectedKeyId} {filter} />
      <button class="add" on:click={() => (showNew = true)}>+ Add a key</button>
    </aside>
    <section>
      {#if $keysLoading && $storedKeys.length === 0}
        <div class="loading">Loading…</div>
      {:else}
        <KeyDetails
          key={selected}
          {usages}
          {copied}
          on:copy={copyPublicKey}
          on:rename={onRename}
          on:policy={openPolicy}
          on:passphrase={() => { dialogError = ''; showPassphrase = true; }}
          on:export={() => { dialogError = ''; showExport = true; }}
          on:deploy={() => { dialogError = ''; showDeploy = true; }}
          on:delete={onDelete}
        />
      {/if}
    </section>
  </div>
</Modal>

<NewKeyDialog bind:show={showNew} on:submit={onCreate} on:close={() => (showNew = false)} />
<ExportKeyDialog bind:show={showExport} target={selected} error={dialogError} on:submit={onExport} on:close={() => (showExport = false)} />
<KeyPassphraseDialog bind:show={showPassphrase} target={selected} error={dialogError} on:submit={onPassphrase} on:close={() => (showPassphrase = false)} />
<DeployKeyDialog bind:show={showDeploy} target={selected} sessions={$sessions} error={dialogError} {busy} on:submit={onDeploy} on:close={() => (showDeploy = false)} />

<Modal title="Settings for {selected?.comment || 'key'}" show={showPolicy} on:close={() => (showPolicy = false)}>
  <KeyPolicyFields bind:options={policyDraft} showNonExportable={!selected?.nonExportable} hasPassphrase={selected?.policy === 'passphrase'} />
  <div class="dialog-actions">
    <button on:click={() => (showPolicy = false)}>Cancel</button>
    <button class="primary" on:click={savePolicy}>Save</button>
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

  .layout {
    display: grid;
    grid-template-columns: minmax(200px, 260px) 1fr;
    min-height: 380px;
    max-height: 60vh;
  }

  aside {
    display: flex;
    flex-direction: column;
    border-right: 1px solid var(--border, rgba(255, 255, 255, 0.12));
  }

  .search {
    margin: 0 8px 8px;
    padding: 5px 8px;
    font-size: 12px;
    border-radius: 4px;
    border: 1px solid var(--border, rgba(255, 255, 255, 0.14));
    background: var(--bg-input, rgba(0, 0, 0, 0.25));
    color: var(--text-primary, #ddd);
  }

  .add {
    margin: 8px;
    padding: 6px;
    font-size: 12px;
    border-radius: 4px;
    border: 1px dashed var(--border, rgba(255, 255, 255, 0.2));
    background: transparent;
    color: var(--text-secondary, #aaa);
    cursor: pointer;
  }

  .add:hover {
    color: var(--text-primary, #ddd);
    border-color: var(--accent, #4a9eff);
  }

  section {
    overflow-y: auto;
  }

  .loading {
    padding: 20px;
    font-size: 12px;
    color: var(--text-secondary, #888);
  }

  button.primary {
    background: var(--accent, #4a9eff);
    color: #fff;
    border-color: transparent;
  }
</style>
