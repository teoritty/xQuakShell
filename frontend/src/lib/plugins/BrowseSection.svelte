<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { RefreshCw, Download } from 'lucide-svelte';
  import PluginCard from './PluginCard.svelte';
  import { buildBrowseGroups, canInstallFrom, sourceStatus } from './pluginsView';
  import { defaultReleaseTagForPlugin, githubPluginStatusLabel } from '../pluginDisplay';
  import type { PluginSourceDTO } from '../../api/pluginSources';
  import type { GitHubPluginMetadata } from '../../api/githubPlugins';

  export let sources: PluginSourceDTO[] = [];
  export let pluginsBySource: Record<string, GitHubPluginMetadata[]> = {};
  export let errorsBySource: Record<string, string> = {};
  export let loadingSources: Record<string, boolean> = {};
  export let selectedTags: Record<string, string> = {};
  export let query = '';
  export let busyPluginId = '';

  const dispatch = createEventDispatcher();

  $: groups = buildBrowseGroups(sources, pluginsBySource, errorsBySource, loadingSources, query);

  function tagFor(plugin: GitHubPluginMetadata): string {
    return selectedTags[plugin.id] ?? defaultReleaseTagForPlugin(plugin);
  }

  // A release that has no asset for this host cannot be installed, and saying so on the option
  // itself is what stops a user picking one and only then being refused.
  function tagLabel(release: { tag: string; prerelease: boolean; platformSupported: boolean }): string {
    if (!release.platformSupported) return `${release.tag} (unsupported here)`;
    return release.prerelease ? `${release.tag} (pre-release)` : release.tag;
  }
</script>

{#if groups.length === 0}
  <p class="empty">
    {query ? `Nothing matches “${query}”.` : 'No source has anything to offer yet.'}
  </p>
{:else}
  {#each groups as group (group.source.id)}
    {@const status = sourceStatus(group.source)}
    <section class="group">
      <header class="group-head">
        <span class="group-name">{group.source.displayName}</span>
        <span class="group-status" class:bad={status.kind !== 'trusted'}>{status.text}</span>
        {#if canInstallFrom(group.source)}
          <button
            class="ghost icon-btn"
            title="Refresh"
            disabled={group.loading}
            on:click={() => dispatch('refreshSource', { source: group.source })}
          >
            <RefreshCw size={12} />
          </button>
        {/if}
      </header>

      {#if group.loading}
        <p class="note">Loading…</p>
      {:else if group.error}
        <p class="note error">{group.error}</p>
      {:else if !group.source.available}
        <p class="note">{group.source.unavailableReason || 'This source is unavailable.'}</p>
      {:else}
        <div class="card-list">
          {#each group.plugins as plugin (plugin.id)}
            {@const label = githubPluginStatusLabel(plugin)}
            <PluginCard
              name={plugin.name}
              version={plugin.version}
              description={plugin.description}
              origin={plugin.author}
              status={label.text}
              statusKind={label.kind}
              showDetails={true}
              on:details={() => dispatch('details', { plugin, source: group.source })}
            >
              <svelte:fragment slot="actions">
                {#if plugin.availableReleases?.length}
                  <select
                    class="tag-select"
                    value={tagFor(plugin)}
                    on:change={(e) =>
                      dispatch('selectTag', { pluginId: plugin.id, tag: e.currentTarget.value })}
                  >
                    {#each plugin.availableReleases as release (release.tag)}
                      <option value={release.tag}>{tagLabel(release)}</option>
                    {/each}
                  </select>
                {/if}
                <button
                  class="primary small"
                  disabled={busyPluginId === plugin.id || !plugin.platformSupported}
                  title={plugin.platformSupported ? '' : 'No release asset targets this platform'}
                  on:click={() =>
                    dispatch('install', {
                      source: group.source,
                      plugin,
                      releaseTag: tagFor(plugin),
                    })}
                >
                  <Download size={12} />
                  {plugin.installed ? 'Reinstall' : 'Install'}
                </button>
              </svelte:fragment>
            </PluginCard>
          {/each}
        </div>
      {/if}
    </section>
  {/each}
{/if}

<style>
  .group {
    margin-bottom: 16px;
  }

  .group-head {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 6px;
  }

  .group-name {
    font-size: 12px;
    font-weight: 600;
  }

  .group-status {
    font-size: 11px;
    color: var(--success, #3fb950);
  }

  .group-status.bad {
    color: var(--warning, #d29922);
  }

  .card-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .note {
    margin: 0;
    font-size: 12px;
    color: var(--text-secondary);
  }

  .note.error {
    color: var(--danger, #f85149);
  }

  .empty {
    margin: 0;
    font-size: 12px;
    color: var(--text-secondary);
  }

  .icon-btn {
    display: inline-flex;
    align-items: center;
    padding: 3px;
  }

  .tag-select {
    font-size: 11px;
    max-width: 160px;
  }

  .small {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 11px;
    padding: 3px 8px;
  }
</style>
