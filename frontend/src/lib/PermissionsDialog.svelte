<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { X, Shield } from 'lucide-svelte';
  import { chmodPath, chmodPathRecursive, chownPath, chownPathRecursive, type ApplyTarget } from '../api/remoteFs';

  export let show = false;
  export let sessionId: string;
  export let path = '';
  export let isDir = false;
  export let currentMode = ''; // e.g. "-rwxr-xr-x" (Go FileMode.String())

  const dispatch = createEventDispatcher<{ close: void }>();

  // Parses a Go-style FileMode string ("-rwxr-xr-x", "drwxr-xr-x", ...) into
  // an octal permission number. Falls back to 0o644 if unparsable.
  function parseModeString(s: string): number {
    if (s.length < 10) return 0o644;
    const bits = s.slice(1); // drop the type char (d/-/l/...)
    let mode = 0;
    const flags = [0o400, 0o200, 0o100, 0o040, 0o020, 0o010, 0o004, 0o002, 0o001];
    for (let i = 0; i < 9; i++) {
      if (bits[i] !== '-') mode |= flags[i];
    }
    return mode;
  }

  let initialMode = 0o644;
  let mode = 0o644;
  let octalText = '644';
  let recurse = false;
  let applyTo: ApplyTarget = 'both';
  let uidInput = '';
  let gidInput = '';
  let applying = false;

  $: if (show) {
    initialMode = parseModeString(currentMode);
    mode = initialMode;
    recurse = false;
    applyTo = 'both';
    uidInput = '';
    gidInput = '';
  }

  $: octalText = mode.toString(8).padStart(3, '0');

  function toggleBit(bit: number) {
    mode = mode ^ bit;
  }

  function commitOctalText() {
    const parsed = parseInt(octalText.replace(/^0o?/, ''), 8);
    if (!isNaN(parsed) && parsed >= 0 && parsed <= 0o777) {
      mode = parsed;
    } else {
      octalText = mode.toString(8).padStart(3, '0');
    }
  }

  $: ownerR = !!(mode & 0o400);
  $: ownerW = !!(mode & 0o200);
  $: ownerX = !!(mode & 0o100);
  $: groupR = !!(mode & 0o040);
  $: groupW = !!(mode & 0o020);
  $: groupX = !!(mode & 0o010);
  $: otherR = !!(mode & 0o004);
  $: otherW = !!(mode & 0o002);
  $: otherX = !!(mode & 0o001);

  function close() {
    dispatch('close');
  }

  async function handleApply() {
    applying = true;
    try {
      if (mode !== initialMode) {
        if (recurse && isDir) {
          await chmodPathRecursive(sessionId, path, mode, applyTo);
        } else {
          await chmodPath(sessionId, path, mode);
        }
      }
      const uidStr = uidInput.trim();
      const gidStr = gidInput.trim();
      if (uidStr !== '' && gidStr !== '') {
        const uid = parseInt(uidStr, 10);
        const gid = parseInt(gidStr, 10);
        if (!isNaN(uid) && !isNaN(gid)) {
          if (recurse && isDir) {
            await chownPathRecursive(sessionId, path, uid, gid, applyTo);
          } else {
            await chownPath(sessionId, path, uid, gid);
          }
        }
      }
    } finally {
      applying = false;
      close();
    }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Escape') close();
  }
</script>

{#if show}
  <div class="confirm-backdrop" on:click={close} on:keydown={handleKeydown}>
    <div class="perm-dialog" on:click|stopPropagation on:keydown|stopPropagation>
      <div class="confirm-header">
        <span class="confirm-title"><Shield size={16} /> Permissions</span>
        <button class="confirm-close" on:click={close}><X size={14} /></button>
      </div>
      <div class="confirm-body">
        <p class="perm-path" title={path}>{path}</p>

        <div class="perm-grid">
          <span></span>
          <span class="perm-col-label">Read</span>
          <span class="perm-col-label">Write</span>
          <span class="perm-col-label">Execute</span>

          <span class="perm-row-label">Owner</span>
          <input type="checkbox" checked={ownerR} on:change={() => toggleBit(0o400)} />
          <input type="checkbox" checked={ownerW} on:change={() => toggleBit(0o200)} />
          <input type="checkbox" checked={ownerX} on:change={() => toggleBit(0o100)} />

          <span class="perm-row-label">Group</span>
          <input type="checkbox" checked={groupR} on:change={() => toggleBit(0o040)} />
          <input type="checkbox" checked={groupW} on:change={() => toggleBit(0o020)} />
          <input type="checkbox" checked={groupX} on:change={() => toggleBit(0o010)} />

          <span class="perm-row-label">Other</span>
          <input type="checkbox" checked={otherR} on:change={() => toggleBit(0o004)} />
          <input type="checkbox" checked={otherW} on:change={() => toggleBit(0o002)} />
          <input type="checkbox" checked={otherX} on:change={() => toggleBit(0o001)} />
        </div>

        <label class="perm-field">
          <span>Octal</span>
          <input
            class="perm-octal-input"
            type="text"
            bind:value={octalText}
            on:blur={commitOctalText}
            on:keydown={(e) => e.key === 'Enter' && commitOctalText()}
          />
        </label>

        {#if isDir}
          <label class="perm-checkbox">
            <input type="checkbox" bind:checked={recurse} />
            Recurse into subdirectories
          </label>
          {#if recurse}
            <div class="perm-apply-to">
              <span>Apply to:</span>
              <label><input type="radio" bind:group={applyTo} value="files" /> Files only</label>
              <label><input type="radio" bind:group={applyTo} value="dirs" /> Directories only</label>
              <label><input type="radio" bind:group={applyTo} value="both" /> Both</label>
            </div>
          {/if}
        {/if}

        <div class="perm-owner-fields">
          <label class="perm-field">
            <span>UID</span>
            <input class="perm-id-input" type="number" bind:value={uidInput} placeholder="unchanged" />
          </label>
          <label class="perm-field">
            <span>GID</span>
            <input class="perm-id-input" type="number" bind:value={gidInput} placeholder="unchanged" />
          </label>
        </div>
      </div>
      <div class="confirm-footer">
        <button class="secondary" on:click={close}>Cancel</button>
        <button class="primary" on:click={handleApply} disabled={applying}>Apply</button>
      </div>
    </div>
  </div>
{/if}

<style>
  .confirm-backdrop {
    position: fixed;
    inset: 0;
    background: rgba(0, 0, 0, 0.6);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 2000;
  }

  .perm-dialog {
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 6px;
    min-width: 340px;
    max-width: 440px;
    box-shadow: 0 8px 32px rgba(0, 0, 0, 0.5);
  }

  .confirm-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 12px 16px;
    border-bottom: 1px solid var(--border-color);
  }

  .confirm-title {
    font-size: 14px;
    font-weight: 600;
    color: var(--text-bright);
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .confirm-close {
    background: transparent;
    color: var(--text-secondary);
    padding: 2px 6px;
    border: none;
    cursor: pointer;
    border-radius: 2px;
  }
  .confirm-close:hover {
    color: var(--text-bright);
    background: var(--bg-hover);
  }

  .confirm-body {
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .perm-path {
    font-size: 11px;
    color: var(--text-secondary);
    margin: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .perm-grid {
    display: grid;
    grid-template-columns: 60px repeat(3, 1fr);
    align-items: center;
    gap: 6px 8px;
    font-size: 12px;
  }

  .perm-col-label,
  .perm-row-label {
    color: var(--text-secondary);
  }

  .perm-field {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    color: var(--text-primary);
  }

  .perm-octal-input {
    width: 60px;
    padding: 3px 6px;
    background: var(--bg-tertiary);
    border: 1px solid var(--border-color);
    border-radius: 3px;
    color: var(--text-primary);
    font-family: monospace;
  }

  .perm-checkbox {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    color: var(--text-primary);
    cursor: pointer;
  }

  .perm-apply-to {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: 10px;
    font-size: 12px;
    color: var(--text-primary);
    padding-left: 16px;
  }
  .perm-apply-to label {
    display: flex;
    align-items: center;
    gap: 4px;
    cursor: pointer;
  }

  .perm-owner-fields {
    display: flex;
    gap: 16px;
  }

  .perm-id-input {
    width: 90px;
    padding: 3px 6px;
    background: var(--bg-tertiary);
    border: 1px solid var(--border-color);
    border-radius: 3px;
    color: var(--text-primary);
  }

  .confirm-footer {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    padding: 12px 16px;
    border-top: 1px solid var(--border-color);
  }
  .confirm-footer button {
    padding: 5px 14px;
    font-size: 12px;
    border: none;
    border-radius: 3px;
    cursor: pointer;
  }
  .confirm-footer .secondary {
    background: var(--bg-tertiary);
    color: var(--text-primary);
  }
  .confirm-footer .secondary:hover {
    background: var(--bg-hover);
  }
  .confirm-footer .primary {
    background: var(--accent);
    color: #fff;
  }
  .confirm-footer .primary:hover {
    opacity: 0.9;
  }
  .confirm-footer .primary:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
</style>
