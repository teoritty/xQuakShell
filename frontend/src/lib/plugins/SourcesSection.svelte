<script lang="ts">
  import { t, currentLocale } from '../../i18n/messages';
  import { createEventDispatcher } from 'svelte';
  import { Plus, RefreshCw, Trash2 } from 'lucide-svelte';
  import SectionHeading from './SectionHeading.svelte';
  import { forgeSources, filterSources, sourceStatus } from './pluginsView';
  import type { PluginSourceDTO } from '../../api/pluginSources';

  export let sources: PluginSourceDTO[] = [];
  export let loadingSources: Record<string, boolean> = {};
  export let busy = false;
  export let query = '';

  const dispatch = createEventDispatcher();

  // Two counts, because they answer different questions: the heading says how many repositories
  // are registered, and the empty state has to tell "you have none" apart from "none match".
  $: registered = forgeSources(sources);
  $: repositories = filterSources(sources, query);

  function fetchedLabel(source: PluginSourceDTO): string {
    if (!source.lastFetchedAt) return $t('plugins.sources.neverFetched');
    const when = new Date(source.lastFetchedAt);
    return Number.isNaN(when.getTime())
      ? $t('plugins.sources.neverFetched')
      : $t('plugins.sources.fetchedAt', { when: when.toLocaleString($currentLocale) });
  }
</script>

<SectionHeading
  title={$t('plugins.section.sources')}
  subtitle={$t('plugins.sources.subtitle')}
  count={query
    ? $t('plugins.count.matched', { matched: repositories.length, total: registered.length })
    : registered.length
      ? String(registered.length)
      : ''}
>
  <button slot="action" class="secondary small" disabled={busy} on:click={() => dispatch('addSource')}>
    <Plus size={12} />
    {$t('plugins.sources.add')}
  </button>
</SectionHeading>

<p class="trust-note">{$t('security.plugin.source.trustNote')}</p>

{#if registered.length > 0 && repositories.length === 0}
  <p class="no-match">{$t('plugins.noMatch', { query })}</p>
{:else if registered.length === 0}
  <div class="empty">
    <p class="empty-title">{$t('plugins.sources.empty.title')}</p>
    <p class="empty-body">{$t('plugins.sources.empty.body')}</p>
    <button class="secondary" disabled={busy} on:click={() => dispatch('addSource')}>
      <Plus size={12} />
      {$t('plugins.sources.add')}
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
            <span class="source-status status-{status.kind}">
              {status.textKey ? $t(status.textKey) : status.text}
            </span>
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
            {$t('security.plugin.source.trusted')}
          </label>
          <button
            class="ghost icon-btn"
            title={$t('common.refresh')}
            disabled={busy || loadingSources[source.id]}
            on:click={() => dispatch('refreshSource', { source })}
          >
            <RefreshCw size={13} />
          </button>
          <button
            class="ghost icon-btn danger"
            title={$t('plugins.sources.remove')}
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
  .no-match {
    margin: 0;
    font-size: 12px;
    color: var(--text-secondary);
  }

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
