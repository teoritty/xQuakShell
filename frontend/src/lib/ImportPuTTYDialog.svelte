<script lang="ts">
  import Modal from './Modal.svelte';
  import { importPuTTYPPK, importPuTTYRegPreview, importPuTTYRegAsConnections, type PuTTYSessionPreview } from '../api/credentials';
  import { refreshAllConnections, refreshIdentities } from '../actions/connectionActions';
  import { creationTargetFolderId } from '../stores/appState';
  import { bytesToBase64, decodeTextFile } from './importPuTTY/fileEncoding';
  import { KeyRound } from 'lucide-svelte';

  export let show = false;

  let importing = false;
  let fileType: 'ppk' | 'reg' | null = null;
  let ppkPassphrase = '';
  let ppkBase64 = '';
  let regPreview: PuTTYSessionPreview[] = [];
  let regContent = '';
  let importResult = '';

  $: if (show) {
    fileType = null;
    regPreview = [];
    regContent = '';
    ppkBase64 = '';
    importResult = '';
  }

  // Both branches read raw bytes rather than File.text(): that helper always
  // assumes UTF-8, and neither of these files is reliably UTF-8. See
  // importPuTTY/fileEncoding.ts.
  async function handleFileSelect(e: Event) {
    const input = e.target as HTMLInputElement;
    const file = input.files?.[0];
    if (!file) return;
    const name = file.name.toLowerCase();
    const bytes = new Uint8Array(await file.arrayBuffer());
    if (name.endsWith('.ppk')) {
      fileType = 'ppk';
      ppkBase64 = bytesToBase64(bytes);
    } else if (name.endsWith('.reg')) {
      fileType = 'reg';
      regContent = decodeTextFile(bytes);
      regPreview = await importPuTTYRegPreview(regContent);
    }
    input.value = '';
  }

  async function doImportPPK() {
    if (!ppkBase64) return;
    importing = true;
    try {
      const id = await importPuTTYPPK(ppkBase64, ppkPassphrase);
      if (id) {
        importResult = `Key imported. ID: ${id.slice(0, 8)}...`;
        await refreshIdentities();
      }
    } finally {
      importing = false;
    }
  }

  async function doImportReg() {
    if (!regContent) return;
    importing = true;
    try {
      const folderId = $creationTargetFolderId || '';
      const conns = await importPuTTYRegAsConnections(regContent, folderId);
      importResult = `Imported ${conns.length} connection(s).`;
      await refreshAllConnections();
    } finally {
      importing = false;
    }
  }
</script>

{#if show}
  <Modal title="Import from PuTTY" show={show} on:close={() => show = false}>
    <div class="import-body">
      <p class="import-desc">Import .ppk (private key) or .reg (saved sessions).</p>
      <label class="file-drop">
        <input type="file" accept=".ppk,.reg" on:change={handleFileSelect} hidden />
        <div class="drop-zone">
          <KeyRound size={24} />
          <span>Select .ppk or .reg file</span>
        </div>
      </label>

      {#if fileType === 'ppk'}
        <label class="field-block">
          <span>Passphrase (if key is encrypted)</span>
          <input type="password" bind:value={ppkPassphrase} placeholder="Leave empty if unencrypted" />
        </label>
        <button class="primary" on:click={doImportPPK} disabled={importing}>
          {importing ? 'Importing...' : 'Import key'}
        </button>
      {/if}

      {#if fileType === 'reg' && regPreview.length === 0}
        <!-- Without this the dialog rendered nothing at all for a .reg that
             yielded no sessions, which reads as the file picker having failed. -->
        <p class="reg-empty">
          No saved sessions found in this file. A PuTTY export should contain
          <code>[…\SimonTatham\PuTTY\Sessions\…]</code> entries with a
          <code>HostName</code> value.
        </p>
      {/if}

      {#if fileType === 'reg' && regPreview.length > 0}
        <div class="reg-preview">
          <h4>Found {regPreview.length} session(s)</h4>
          <div class="preview-list">
            {#each regPreview as s}
              <div class="preview-item">{s.name} — {s.hostName}:{s.port} {#if s.userName}({s.userName}){/if}</div>
            {/each}
          </div>
          <button class="primary" on:click={doImportReg} disabled={importing}>
            {importing ? 'Importing...' : 'Import to current folder'}
          </button>
        </div>
      {/if}

      {#if importResult}
        <p class="import-result">{importResult}</p>
      {/if}
    </div>
  </Modal>
{/if}

<style>
  .import-body { padding: 8px 0; }
  .import-desc { font-size: 12px; color: var(--text-secondary); margin-bottom: 12px; }
  .file-drop { cursor: pointer; display: block; }
  .drop-zone {
    border: 2px dashed var(--border-color);
    border-radius: 6px;
    padding: 24px;
    text-align: center;
    color: var(--text-secondary);
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
  }
  .drop-zone:hover { border-color: var(--accent); color: var(--text-primary); }
  .field-block { display: flex; flex-direction: column; gap: 4px; margin-top: 12px; }
  .field-block span { font-size: 11px; color: var(--text-secondary); }
  .reg-empty { font-size: 12px; color: var(--text-secondary); margin-top: 12px; }
  .reg-empty code { font-size: 11px; color: var(--text-primary); }
  .reg-preview { margin-top: 16px; }
  .reg-preview h4 { font-size: 12px; margin-bottom: 8px; }
  .preview-list { max-height: 120px; overflow-y: auto; margin-bottom: 12px; font-size: 11px; }
  .preview-item { padding: 2px 0; color: var(--text-secondary); }
  .import-result { font-size: 12px; color: var(--accent); margin-top: 12px; }
</style>
