<script lang="ts">
  import Modal from './Modal.svelte';
  import KeyList from './keys/KeyList.svelte';
  import KeyDetails from './keys/KeyDetails.svelte';
  import NewKeyDialog from './keys/NewKeyDialog.svelte';
  import ExportKeyDialog from './keys/ExportKeyDialog.svelte';
  import KeyPassphraseDialog from './keys/KeyPassphraseDialog.svelte';
  import DeployKeyDialog from './keys/DeployKeyDialog.svelte';
  import RenameKeyDialog from './keys/RenameKeyDialog.svelte';
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
  let showRename = false;
  let policyDraft: KeyOptions = { ...defaultKeyOptions };
  let dialogError = '';
  let deployNotice = '';
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

  async function onRename(event: CustomEvent) {
    if (!selected) return;
    await saveKeyName(selected.id, event.detail);
    showRename = false;
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
        deployNotice = result.alreadyPresent
          ? 'That key was already authorised on this server — nothing was changed.'
          : `Added to ${result.path}.`;
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
      <div class="rail-head">
        <input class="search" placeholder="Search keys" bind:value={filter} />
      </div>
      <KeyList keys={$storedKeys} bind:selectedId={$selectedKeyId} {filter} />
      <div class="rail-foot">
        <button class="secondary" on:click={() => (showNew = true)}>Add a key</button>
      </div>
    </aside>
    <section class="pane">
      {#if $keysLoading && $storedKeys.length === 0}
        <div class="loading">Loading keys…</div>
      {:else}
        <KeyDetails
          key={selected}
          {usages}
          {copied}
          on:copy={copyPublicKey}
          on:rename={() => (showRename = true)}
          on:policy={openPolicy}
          on:passphrase={() => { dialogError = ''; showPassphrase = true; }}
          on:export={() => { dialogError = ''; showExport = true; }}
          on:deploy={() => { dialogError = ''; deployNotice = ''; showDeploy = true; }}
          on:delete={onDelete}
        />
      {/if}
    </section>
  </div>
</Modal>

<RenameKeyDialog bind:show={showRename} target={selected} on:submit={onRename} on:close={() => (showRename = false)} />
<NewKeyDialog bind:show={showNew} on:submit={onCreate} on:close={() => (showNew = false)} />
<ExportKeyDialog bind:show={showExport} target={selected} error={dialogError} on:submit={onExport} on:close={() => (showExport = false)} />
<KeyPassphraseDialog bind:show={showPassphrase} target={selected} error={dialogError} on:submit={onPassphrase} on:close={() => (showPassphrase = false)} />
<DeployKeyDialog bind:show={showDeploy} target={selected} sessions={$sessions} error={dialogError} notice={deployNotice} {busy} on:submit={onDeploy} on:close={() => (showDeploy = false)} />

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



  .layout {
    display: grid;
    grid-template-columns: 288px 1fr;
    flex: 1;
    min-height: 0;
  }

  aside {
    display: flex;
    flex-direction: column;
    min-height: 0;
    border-right: 1px solid var(--border-color);
    background: var(--bg-primary);
  }

  .rail-head {
    padding: 10px 10px 8px;
  }

  .rail-foot {
    padding: 8px 10px;
    border-top: 1px solid var(--border-color);
  }

  .rail-foot button {
    width: 100%;
  }

  .search {
    width: 100%;
  }



  .pane {
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  .loading {
    padding: 28px 20px;
    font-size: 12px;
    color: var(--text-secondary);
  }

</style>
