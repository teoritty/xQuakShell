<script lang="ts">
  import Modal from './Modal.svelte';
  import { conflictRequest, respondConflict } from '../stores/conflictPrompt';
  import { CONFLICT_ACTIONS, type ConflictAction } from './transfer/conflictActions';
  import type { ConflictInfoDTO, PlannedFileDTO } from '../backend/gateway';

  // Local editable state, reset whenever a new conflict is shown.
  let action: ConflictAction = 'overwrite';
  let newName = '';
  let applyToAll = false;
  // FileZilla's "Apply to current queue only": checked = do NOT persist as
  // default. We store the inverse (rememberDefault) and bind the checkbox to it.
  let queueOnly = true;

  let current: PlannedFileDTO | null = null;

  // Re-initialise the form each time a new request arrives.
  $: if ($conflictRequest && $conflictRequest.file !== current) {
    current = $conflictRequest.file;
    action = 'overwrite';
    newName = suggestRename(baseName(current.target));
    applyToAll = false;
    queueOnly = true;
  }

  function baseName(p: string): string {
    const parts = p.split(/[\\/]/).filter(Boolean);
    return parts.length ? parts[parts.length - 1] : p;
  }

  // suggestRename inserts " (1)" before the extension, matching the backend's
  // NextAvailableName numbering (the backend still guarantees uniqueness).
  function suggestRename(name: string): string {
    const dot = name.lastIndexOf('.');
    if (dot <= 0) return `${name} (1)`;
    return `${name.slice(0, dot)} (1)${name.slice(dot)}`;
  }

  function formatBytes(n: number): string {
    if (n < 1024) return `${n} B`;
    const units = ['KB', 'MB', 'GB', 'TB'];
    let v = n / 1024;
    let i = 0;
    while (v >= 1024 && i < units.length - 1) {
      v /= 1024;
      i++;
    }
    return `${v.toFixed(1)} ${units[i]}`;
  }

  function formatDate(iso: string): string {
    if (!iso) return '—';
    const d = new Date(iso);
    return Number.isNaN(d.getTime()) ? iso : d.toLocaleString();
  }

  function onOk() {
    respondConflict({
      action,
      newName: action === 'rename' ? newName.trim() : undefined,
      applyToAll,
      rememberDefault: !queueOnly,
    });
  }

  function onCancel() {
    respondConflict(null);
  }

  function sourceStat(file: PlannedFileDTO): { size: number; date: string } {
    return { size: file.size, date: file.srcModTime };
  }
  function targetStat(c: ConflictInfoDTO | undefined): { size: number; date: string; isDir: boolean } {
    return { size: c?.size ?? 0, date: c?.modTime ?? '', isDir: c?.isDir ?? false };
  }
</script>

{#if $conflictRequest}
  {@const file = $conflictRequest.file}
  {@const src = sourceStat(file)}
  {@const tgt = targetStat(file.conflict)}
  <Modal show={true} title="Target file already exists" on:close={onCancel}>
    <div class="conflict">
      <p class="lead">
        The target {tgt.isDir ? 'path exists as a directory' : 'file already exists'}. Please choose an action.
        {#if $conflictRequest.total > 1}
          <span class="counter">Conflict {$conflictRequest.index + 1} of {$conflictRequest.total}</span>
        {/if}
      </p>

      <div class="files">
        <div class="file-box">
          <div class="file-box-title">Source file</div>
          <div class="file-path" title={file.source}>{file.source}</div>
          <div class="file-meta">{formatBytes(src.size)}</div>
          <div class="file-meta">{formatDate(src.date)}</div>
        </div>
        <div class="file-box">
          <div class="file-box-title">Target file</div>
          <div class="file-path" title={file.target}>{file.target}</div>
          <div class="file-meta">{tgt.isDir ? 'directory' : formatBytes(tgt.size)}</div>
          <div class="file-meta">{formatDate(tgt.date)}</div>
        </div>
      </div>

      <fieldset class="actions">
        <legend>Action</legend>
        {#each CONFLICT_ACTIONS as a}
          <label class="radio">
            <input type="radio" bind:group={action} value={a.value} />
            <span>{a.label}</span>
          </label>
          {#if a.value === 'rename' && action === 'rename'}
            <input class="rename-input" type="text" bind:value={newName} placeholder="New name" />
          {/if}
        {/each}
      </fieldset>

      <label class="check">
        <input type="checkbox" bind:checked={applyToAll} />
        <span>Always use this action</span>
      </label>
      <label class="check">
        <input type="checkbox" bind:checked={queueOnly} />
        <span>Apply to current queue only</span>
      </label>

      <div class="buttons">
        <button class="secondary" on:click={onCancel}>Cancel</button>
        <button class="primary" on:click={onOk} disabled={action === 'rename' && newName.trim() === ''}>OK</button>
      </div>
    </div>
  </Modal>
{/if}

<style>
  .conflict {
    display: flex;
    flex-direction: column;
    gap: 12px;
    min-width: 420px;
  }

  .lead {
    margin: 0;
    font-size: 12px;
    color: var(--text-primary);
  }

  .counter {
    display: block;
    margin-top: 4px;
    color: var(--text-secondary);
    font-size: 11px;
  }

  .files {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 10px;
  }

  .file-box {
    border: 1px solid var(--border-color);
    border-radius: 4px;
    padding: 8px;
    background: var(--bg-primary);
    min-width: 0;
  }

  .file-box-title {
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.4px;
    color: var(--text-secondary);
    margin-bottom: 4px;
  }

  .file-path {
    font-size: 11px;
    font-family: var(--font-mono);
    color: var(--text-bright);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .file-meta {
    font-size: 11px;
    color: var(--text-secondary);
    font-variant-numeric: tabular-nums;
  }

  .actions {
    border: 1px solid var(--border-color);
    border-radius: 4px;
    padding: 8px 10px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .actions legend {
    font-size: 11px;
    font-weight: 600;
    color: var(--text-secondary);
    padding: 0 4px;
  }

  .radio,
  .check {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--text-primary);
    cursor: pointer;
  }

  .rename-input {
    margin: 2px 0 4px 22px;
    padding: 3px 6px;
    font-size: 12px;
    background: var(--bg-primary);
    color: var(--text-primary);
    border: 1px solid var(--border-color);
    border-radius: 3px;
    width: calc(100% - 22px);
  }

  .buttons {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 4px;
  }

  .buttons button {
    padding: 5px 14px;
    font-size: 12px;
  }
</style>
