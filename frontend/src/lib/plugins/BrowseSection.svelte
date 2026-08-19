<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { RefreshCw, Download, ArrowUpCircle } from 'lucide-svelte';
  import PluginCard from './PluginCard.svelte';
  import SectionHeading from './SectionHeading.svelte';
  import { buildBrowseGroups, forgeSources, hasUpdate, sourceStatus } from './pluginsView';
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

  $: repositories = forgeSources(sources);
  $: groups = buildBrowseGroups(sources, pluginsBySource, errorsBySource, loadingSources, query);
  $: updateCount = groups.reduce((n, g) => n + g.plugins.filter(hasUpdate).length, 0);

  function tagFor(plugin: GitHubPluginMetadata): string {
    return selectedTags[plugin.id] ?? defaultReleaseTagForPlugin(plugin);
  }

  // A release with no asset for this host cannot be installed, and saying so on the option itself
  // is what stops a user picking one and only then being refused.
  function tagLabel(release: { tag: string; prerelease: boolean; platformSupported: boolean }): string {
    if (!release.platformSupported) return `${release.tag} · unsupported here`;
    return release.prerelease ? `${release.tag} · pre-release` : release.tag;
  }

  function chipsFor(plugin: GitHubPluginMetadata) {
    const chips: { label: string; tone: 'good' | 'warn' | 'bad' | 'neutral' }[] = [];
    if (plugin.license) chips.push({ label: plugin.license, tone: 'neutral' });
    if (!plugin.platformSupported) chips.push({ label: 'Not for this platform', tone: 'bad' });
    if (hasUpdate(plugin)) chips.push({ label: `Update to ${plugin.latestRelease}`, tone: 'good' });
    return chips;
  }
</script>

<SectionHeading
  title="Browse"
  subtitle="Plugins published by the repositories you registered. Installing always asks before granting anything."
  count={updateCount > 0 ? `${updateCount} update${updateCount === 1 ? '' : 's'}` : ''}
/>

{#if repositories.length === 0}
  <div class="empty">
    <p class="empty-title">No repositories registered</p>
    <p class="empty-body">Add one under Sources and its plugins appear here.</p>
    <button class="secondary" on:click={() => dispatch('goToSources')}>Open Sources</button>
  </div>
{:else if groups.length === 0}
  <p class="no-match">{query ? `Nothing matches “${query}”.` : 'No repository has published a plugin yet.'}</p>
{:else}
  {#each groups as group (group.source.id)}
    {@const status = sourceStatus(group.source)}
    <section class="group">
      <header class="group-head">
        <span class="group-name">{group.source.displayName}</span>
        <span class="group-status status-{status.kind}">{status.text}</span>
        <button
          class="ghost icon-btn"
          title="Refresh this repository"
          disabled={group.loading}
          on:click={() => dispatch('refreshSource', { source: group.source })}
        >
          <RefreshCw size={12} />
        </button>
      </header>

      {#if group.loading}
        <p class="note">Loading…</p>
      {:else if group.error}
        <p class="note error">{group.error}</p>
      {:else}
        <div class="card-list">
          {#each group.plugins as plugin (plugin.id)}
            {@const label = githubPluginStatusLabel(plugin)}
            {@const update = hasUpdate(plugin)}
            <PluginCard
              name={plugin.name}
              version={plugin.version}
              description={plugin.description}
              origin={plugin.author ? `by ${plugin.author}` : ''}
              chips={chipsFor(plugin)}
              status={label.text}
              statusKind={update ? 'warning' : label.kind}
              showDetails={true}
              on:details={() => dispatch('details', { plugin, source: group.source })}
            >
              <svelte:fragment slot="actions">
                {#if plugin.availableReleases?.length > 1}
                  <select
                    class="tag-select"
                    title="Release to install"
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
                  {#if update}
                    <ArrowUpCircle size={12} />
                    Update
                  {:else}
                    <Download size={12} />
                    {plugin.installed ? 'Reinstall' : 'Install'}
                  {/if}
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
    margin-bottom: 18px;
  }

  .group-head {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 7px;
    padding-bottom: 5px;
    border-bottom: 1px solid var(--border-color);
  }

  .group-name {
    font-size: 11px;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--text-secondary);
  }

  .group-status {
    font-size: 10px;
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

  .card-list {
    display: flex;
    flex-direction: column;
    gap: 6px;
  }

  .note {
    margin: 0;
    font-size: 12px;
    color: var(--text-secondary);
  }

  .note.error {
    color: var(--danger);
  }

  .no-match {
    margin: 0;
    font-size: 12px;
    color: var(--text-secondary);
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
    margin: 0 0 12px;
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
    max-width: 170px;
  }

  .small {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 11px;
    padding: 3px 9px;
    white-space: nowrap;
  }
</style>
