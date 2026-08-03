<script lang="ts">
  /** Path field for the config file, with a native file picker. */
  import { createEventDispatcher } from 'svelte';
  import { FolderOpen, RefreshCw } from 'lucide-svelte';

  export let path = '';
  export let busy = false;
  /** True once a detection attempt found nothing, so the hint is earned. */
  export let noDefaultFound = false;

  const dispatch = createEventDispatcher<{ browse: void; reload: void }>();
</script>

<div class="source-field">
  <label class="field-label" for="ssh-config-path">SSH config file</label>
  <div class="row">
    <input
      id="ssh-config-path"
      type="text"
      bind:value={path}
      placeholder="~/.ssh/config"
      spellcheck="false"
      autocomplete="off"
      on:keydown={(e) => e.key === 'Enter' && dispatch('reload')}
    />
    <button class="icon-btn" title="Browse…" disabled={busy} on:click={() => dispatch('browse')}>
      <FolderOpen size={14} />
    </button>
    <button
      class="icon-btn"
      title="Read this file"
      disabled={busy || !path.trim()}
      on:click={() => dispatch('reload')}
    >
      <RefreshCw size={14} />
    </button>
  </div>
  {#if noDefaultFound && !path.trim()}
    <p class="hint">
      No config found at <code>~/.ssh/config</code>. Choose a file to import from.
    </p>
  {/if}
</div>

<style>
  .source-field {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .field-label {
    font-size: 11px;
    color: var(--text-secondary);
  }

  .row {
    display: flex;
    gap: 4px;
  }

  input {
    flex: 1;
    min-width: 0;
    font-size: 12px;
    padding: 5px 8px;
    background: var(--bg-input);
    color: var(--text-primary);
    border: 1px solid var(--border-color);
    border-radius: 4px;
    outline: none;
  }

  input:focus {
    border-color: var(--accent);
  }

  .icon-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 0 8px;
    background: var(--bg-input);
    color: var(--text-secondary);
    border: 1px solid var(--border-color);
    border-radius: 4px;
    cursor: pointer;
  }

  .icon-btn:hover:not(:disabled) {
    color: var(--text-primary);
    border-color: var(--accent);
  }

  .icon-btn:disabled {
    opacity: 0.5;
    cursor: default;
  }

  .hint {
    font-size: 11px;
    color: var(--text-secondary);
  }

  code {
    font-size: 11px;
    color: var(--text-primary);
  }
</style>
