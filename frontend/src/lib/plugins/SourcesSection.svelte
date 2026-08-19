<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { Plus, RefreshCw, Trash2 } from 'lucide-svelte';
  import SectionHeading from './SectionHeading.svelte';
  import { forgeSources, sourceStatus } from './pluginsView';
  import type { PluginSourceDTO } from '../../api/pluginSources';

  export let sources: PluginSourceDTO[] = [];
  export let loadingSources: Record<string, boolean> = {};
  export let busy = false;

  const dispatch = createEventDispatcher();

  $: repositories = forgeSources(sources);

  function fetchedLabel(source: PluginSourceDTO): string {
    if (!source.lastFetchedAt) return 'Never fetched';
    const when = new Date(source.lastFetchedAt);
    return Number.isNaN(when.getTime()) ? 'Never fetched' : `Fetched ${when.toLocaleString()}`;
  }
</script>

<SectionHeading
  title="Sources"
  subtitle="Repositories you have registered as places to install plugins from."
  count={repositories.length ? String(repositories.length) : ''}
>
  <button slot="action" class="secondary small" disabled={busy} on:click={() => dispatch('addSource')}>
    <Plus size={12} />
    Add repository
  </button>
</SectionHeading>

<p class="trust-note">
  Trusting a repository removes the extra confirmation before its plugins install. It is a
  statement about the people who publish there — signature, permission and platform checks run
  either way.
</p>

{#if repositories.length === 0}
  <div class="empty">
    <p class="empty-title">No repositories yet</p>
    <p class="empty-body">
      Add a GitHub or GitLab repository that publishes an xqsp.json and releases for your platform.
    </p>
    <button class="secondary" disabled={busy} on:click={() => dispatch('addSource')}>
      <Plus size={12} />
      Add repository
    </button>
  </div>
{:else}
  <div class="source-list">
    {#each repositories as source (source.id)}
      {@const status = sourceStatus(source)}
      <div class="source-row">
        <div class="source-main">
          <div class="source-name">
            {source.displayName}
            <span class="source-status status-{status.kind}">{status.text}</span>
          </div>
          <div class="source-id">{source.id}</div>
          <div class="source-meta">{fetchedLabel(source)}</div>
        </div>

        <div class="source-actions">
          <label class="trust-toggle">
            <input
              type="checkbox"
              checked={source.trusted}
              disabled={busy}
              on:change={(e) => dispatch('setTrust', { source, trusted: e.currentTarget.checked })}
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
          <button
            class="ghost icon-btn danger"
            title="Remove repository"
            disabled={busy}
            on:click={() => dispatch('removeSource', { source })}
          >
            <Trash2 size={13} />
          </button>
        </div>
      </div>
    {/each}
  </div>
{/if}

<style>
  .trust-note {
    margin: 0 0 14px;
    max-width: 64ch;
    font-size: 11.5px;
    line-height: 1.5;
    color: var(--text-secondary);
  }

  .source-list {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .source-row {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
    padding: 11px 13px;
    border: 1px solid var(--border-color);
    border-radius: 5px;
    background: var(--bg-primary);
    transition: border-color 0.12s ease;
  }

  .source-row:hover {
    border-color: var(--border-focus);
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
    gap: 8px;
    font-size: 13px;
    font-weight: 600;
    color: var(--text-bright);
  }

  .source-status {
    font-size: 10px;
    font-weight: 500;
    padding: 0 6px;
    border-radius: 3px;
    border: 1px solid transparent;
  }

  .status-trusted {
    color: var(--success);
    border-color: rgba(90, 158, 94, 0.4);
  }

  .status-untrusted,
  .status-unavailable {
    color: var(--warning);
    border-color: rgba(196, 144, 64, 0.45);
  }

  .source-id {
    font-family: var(--font-mono);
    font-size: 11px;
    color: var(--text-secondary);
    overflow-wrap: anywhere;
  }

  .source-meta {
    font-size: 11px;
    color: var(--text-secondary);
  }

  .source-actions {
    display: flex;
    align-items: center;
    gap: 7px;
    flex-shrink: 0;
  }

  .trust-toggle {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    font-size: 11px;
    white-space: nowrap;
    color: var(--text-secondary);
    cursor: pointer;
  }

  .icon-btn {
    display: inline-flex;
    align-items: center;
    padding: 3px;
  }

  .icon-btn.danger:hover:not(:disabled) {
    color: var(--danger);
  }

  .empty {
    padding: 26px 20px;
    border: 1px dashed var(--border-color);
    border-radius: 6px;
    text-align: center;
  }

  .empty-title {
    margin: 0 0 5px;
    font-size: 13px;
    font-weight: 600;
    color: var(--text-bright);
  }

  .empty-body {
    margin: 0 auto 12px;
    max-width: 48ch;
    font-size: 12px;
    line-height: 1.55;
    color: var(--text-secondary);
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
