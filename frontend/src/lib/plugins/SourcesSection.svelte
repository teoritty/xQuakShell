<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { Plus, RefreshCw, Trash2 } from 'lucide-svelte';
  import { sourceStatus } from './pluginsView';
  import type { PluginSourceDTO } from '../../api/pluginSources';

  export let sources: PluginSourceDTO[] = [];
  export let loadingSources: Record<string, boolean> = {};
  export let busy = false;

  const dispatch = createEventDispatcher();

  function fetchedLabel(source: PluginSourceDTO): string {
    if (!source.lastFetchedAt) return 'Never fetched';
    const when = new Date(source.lastFetchedAt);
    return Number.isNaN(when.getTime()) ? 'Never fetched' : `Fetched ${when.toLocaleString()}`;
  }
</script>

<div class="sources-head">
  <p class="hint">
    Trust decides whether a source's plugins may install without an extra confirmation. It is not
    a check on the plugin itself — signature and permission checks run either way.
  </p>
  <button class="secondary small" disabled={busy} on:click={() => dispatch('addSource')}>
    <Plus size={12} />
    Add repository
  </button>
</div>

<div class="source-list">
  {#each sources as source (source.id)}
    {@const status = sourceStatus(source)}
    <div class="source-row" class:unavailable={!source.available}>
      <div class="source-main">
        <div class="source-name">
          {source.displayName}
          <span class="source-kind">{source.kind === 'marketplace' ? 'Marketplace' : 'Repository'}</span>
        </div>
        <div class="source-id">{source.id}</div>
        <div class="source-meta">
          <span class="source-status" class:bad={status.kind !== 'trusted'}>{status.text}</span>
          {#if source.kind === 'forge'}
            <span class="source-fetched">{fetchedLabel(source)}</span>
          {/if}
        </div>
      </div>

      <div class="source-actions">
        {#if source.available && source.kind === 'forge'}
          <label class="trust-toggle" title="Trusted">
            <input
              type="checkbox"
              checked={source.trusted}
              disabled={busy}
              on:change={(e) =>
                dispatch('setTrust', { source, trusted: e.currentTarget.checked })}
            />
            Trusted
          </label>
          <button
            class="ghost icon-btn"
            title="Refresh"
            disabled={busy || loadingSources[source.id]}
            on:click={() => dispatch('refreshSource', { source })}
          >
            <RefreshCw size={13} />
          </button>
        {/if}
        {#if source.removable}
          <button
            class="ghost icon-btn danger"
            title="Remove"
            disabled={busy}
            on:click={() => dispatch('removeSource', { source })}
          >
            <Trash2 size={13} />
          </button>
        {/if}
      </div>
    </div>
  {/each}
</div>

<style>
  .sources-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
    margin-bottom: 12px;
  }

  .hint {
    margin: 0;
    font-size: 11px;
    color: var(--text-secondary);
    max-width: 46ch;
  }

  .source-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .source-row {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
    padding: 10px 12px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-secondary);
  }

  .source-row.unavailable {
    opacity: 0.7;
  }

  .source-main {
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  .source-name {
    display: flex;
    align-items: center;
    gap: 7px;
    font-size: 13px;
    font-weight: 600;
  }

  .source-kind {
    font-size: 10px;
    font-weight: 500;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--text-secondary);
  }

  .source-id {
    font-size: 11px;
    color: var(--text-secondary);
    overflow-wrap: anywhere;
  }

  .source-meta {
    display: flex;
    gap: 10px;
    font-size: 11px;
    color: var(--text-secondary);
  }

  .source-status {
    color: var(--success, #3fb950);
  }

  .source-status.bad {
    color: var(--warning, #d29922);
  }

  .source-actions {
    display: flex;
    align-items: center;
    gap: 6px;
    flex-shrink: 0;
  }

  .trust-toggle {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 11px;
    white-space: nowrap;
  }

  .icon-btn {
    display: inline-flex;
    align-items: center;
    padding: 3px;
  }

  .icon-btn.danger:hover:not(:disabled) {
    color: var(--danger, #f85149);
  }

  .small {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 11px;
    padding: 4px 9px;
    white-space: nowrap;
  }
</style>
