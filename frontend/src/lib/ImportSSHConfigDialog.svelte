<script lang="ts">
  /**
   * Import connections from an OpenSSH client config.
   *
   * Container: owns the read → select → import flow and talks to the backend.
   * Rendering lives in the importSSHConfig/ children, and the selection rules
   * in importSelection.ts.
   *
   * Nothing is written to the vault until the user confirms, and only the
   * chosen file path and host aliases ever leave the frontend — the backend
   * reads the config and any key files itself.
   */
  import Modal from './Modal.svelte';
  import SSHConfigSourceField from './importSSHConfig/SSHConfigSourceField.svelte';
  import SSHConfigHostList from './importSSHConfig/SSHConfigHostList.svelte';
  import SSHConfigNotices from './importSSHConfig/SSHConfigNotices.svelte';
  import ImportTargetPicker from './importSSHConfig/ImportTargetPicker.svelte';
  import {
    defaultSelection,
    describeResult,
    importButtonLabel,
    selectAll,
    selectedKeyCount,
    toggleAlias
  } from './importSSHConfig/importSelection';
  import {
    fetchSSHConfigDefaultPath,
    importSSHConfig,
    previewSSHConfig,
    type SSHConfigPreview
  } from '../api/sshConfig';
  import { selectLocalFile } from '../api/localFs';
  import { refreshAllConnections, refreshIdentities } from '../actions/connectionActions';
  import { refreshFolders, saveFolder } from '../actions/folderActions';
  import { folders, selectedFolderId } from '../stores/appState';

  export let show = false;

  let path = '';
  let preview: SSHConfigPreview | null = null;
  let selected: Set<string> = new Set();
  let importKeys = false;
  let folderId = '';
  let newFolderName = '';
  let busy = false;
  let resultMessage = '';
  let noDefaultFound = false;
  let initialized = false;

  $: if (show && !initialized) void openDialog();
  $: if (!show && initialized) initialized = false;

  async function openDialog() {
    initialized = true;
    resetState();
    folderId = $selectedFolderId || '';
    busy = true;
    try {
      const detected = await fetchSSHConfigDefaultPath();
      noDefaultFound = !detected;
      if (detected) {
        path = detected;
        await loadPreview();
      }
    } finally {
      busy = false;
    }
  }

  function resetState() {
    path = '';
    preview = null;
    selected = new Set();
    importKeys = false;
    newFolderName = '';
    resultMessage = '';
    noDefaultFound = false;
  }

  async function loadPreview() {
    const target = path.trim();
    if (!target) return;
    busy = true;
    resultMessage = '';
    try {
      const result = await previewSSHConfig(target);
      preview = result;
      selected = defaultSelection(result.hosts);
      // Key import stays opt-in, and only offered when there is something to
      // read: enabling it by default would read key material unasked.
      importKeys = false;
    } finally {
      busy = false;
    }
  }

  async function browse() {
    const chosen = await selectLocalFile();
    if (!chosen) return;
    path = chosen;
    noDefaultFound = false;
    await loadPreview();
  }

  /** Resolves the destination, creating the new folder only now. */
  async function resolveFolderId(): Promise<string | null> {
    const name = newFolderName.trim();
    if (!name) return folderId;
    const created = await saveFolder({ name, parentId: folderId, order: 0 });
    if (!created) return null;
    await refreshFolders();
    return created.id;
  }

  async function runImport() {
    if (!preview || selected.size === 0) return;
    busy = true;
    resultMessage = '';
    try {
      const target = await resolveFolderId();
      if (target === null) return;
      const result = await importSSHConfig(preview.path, [...selected], target, importKeys);
      resultMessage = describeResult(result);
      if (result.connections.length > 0) await refreshAllConnections();
      if (result.importedKeys > 0) await refreshIdentities();
      // The preview stays on screen on purpose: after a partial import the
      // user can adjust the selection and retry without re-picking the file.
      newFolderName = '';
    } finally {
      busy = false;
    }
  }

  $: hosts = preview?.hosts ?? [];
  $: keysInSelection = selectedKeyCount(hosts, selected);
</script>

{#if show}
  <Modal title="Import from SSH config" {show} on:close={() => (show = false)}>
    <div class="import-body">
      <SSHConfigSourceField
        bind:path
        {busy}
        {noDefaultFound}
        on:browse={browse}
        on:reload={loadPreview}
      />

      {#if preview}
        {#if hosts.length === 0}
          <p class="empty">No importable hosts found in this file.</p>
        {:else}
          <SSHConfigHostList
            {hosts}
            {selected}
            on:toggle={({ detail }) => (selected = toggleAlias(selected, detail))}
            on:selectAll={() => (selected = selectAll(hosts))}
            on:selectNone={() => (selected = new Set())}
            on:selectNew={() => (selected = defaultSelection(hosts))}
          />

          <ImportTargetPicker folders={$folders} bind:folderId bind:newFolderName />

          {#if keysInSelection > 0}
            <label class="key-opt">
              <input type="checkbox" bind:checked={importKeys} />
              <span>
                Also import the {keysInSelection} referenced private
                {keysInSelection === 1 ? 'key' : 'keys'} into the vault
              </span>
            </label>
          {/if}
        {/if}

        <SSHConfigNotices notices={preview.notices} />

        {#if hosts.length > 0}
          <button class="primary" disabled={busy || selected.size === 0} on:click={runImport}>
            {busy ? 'Importing…' : importButtonLabel(selected.size)}
          </button>
        {/if}
      {/if}

      {#if resultMessage}
        <p class="result">{resultMessage}</p>
      {/if}
    </div>
  </Modal>
{/if}

<style>
  .import-body {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 8px 0;
    min-width: 460px;
  }

  .empty,
  .result {
    font-size: 12px;
    color: var(--text-secondary);
  }

  .result {
    color: var(--accent);
  }

  .key-opt {
    display: flex;
    align-items: flex-start;
    gap: 6px;
    font-size: 11px;
    color: var(--text-secondary);
    cursor: pointer;
  }

  .primary {
    align-self: flex-start;
    padding: 6px 14px;
    font-size: 12px;
    background: var(--accent);
    color: var(--bg-primary);
    border: none;
    border-radius: 4px;
    cursor: pointer;
  }

  .primary:disabled {
    opacity: 0.5;
    cursor: default;
  }
</style>
