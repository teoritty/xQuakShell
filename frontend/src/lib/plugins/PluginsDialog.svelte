<script lang="ts">
  // The Plugins screen: rail on the left, one section on the right, and the dialogs that overlay
  // both. It owns the screen's data and routes events; every rendering decision belongs to a
  // section, and every multi-RPC sequence to actions/pluginsActions.
  import Modal from '../Modal.svelte';
  import ConfirmDialog from '../ConfirmDialog.svelte';
  import PluginsToolbar from './PluginsToolbar.svelte';
  import InstalledSection from './InstalledSection.svelte';
  import BrowseSection from './BrowseSection.svelte';
  import SourcesSection from './SourcesSection.svelte';
  import MarketplaceSection from './MarketplaceSection.svelte';
  import PluginDetails from './PluginDetails.svelte';
  import AddSourceDialog from './AddSourceDialog.svelte';
  import InstallFlow from './InstallFlow.svelte';
  import {
    buildRail,
    countNeedsAttention,
    forgeSources,
    type PluginsSectionId,
  } from './pluginsView';
  import { Boxes, Compass, GitBranch, Store } from 'lucide-svelte';
  import {
    fetchSourceCatalog,
    loadPluginsScreen,
    removePlugin,
  } from '../../actions/pluginsActions';
  import { listPlugins, pingPlugin, selectPluginBundleFile, selectPluginSourceDir, setPluginEnabled, type PluginInfo } from '../../api/plugins';
  import {
    addGitHubRepository,
    removeGitHubRepository,
    setGitHubRepositoryTrust,
    type GitHubPluginMetadata,
  } from '../../api/githubPlugins';
  import { listPluginSources, type PluginSourceDTO } from '../../api/pluginSources';

  export let show = false;

  let section: PluginsSectionId = 'installed';
  let query = '';
  let errorMessage = '';
  let busy = false;
  let busyPluginId = '';

  let plugins: PluginInfo[] = [];
  let sources: PluginSourceDTO[] = [];
  let pluginsBySource: Record<string, GitHubPluginMetadata[]> = {};
  let errorsBySource: Record<string, string> = {};
  let loadingSources: Record<string, boolean> = {};
  let selectedTags: Record<string, string> = {};

  let installFlow: InstallFlow;
  let addSourceOpen = false;
  let addSourceBusy = false;
  let detailsPlugin: GitHubPluginMetadata | null = null;
  let detailsSource: PluginSourceDTO | null = null;
  let uninstallTarget: PluginInfo | null = null;
  let uninstallRemoveData = false;

  let loadedOnce = false;
  $: if (show && !loadedOnce) {
    loadedOnce = true;
    void load();
  }
  $: if (!show) loadedOnce = false;

  const RAIL_ICONS = {
    installed: Boxes,
    browse: Compass,
    sources: GitBranch,
    marketplace: Store,
  };

  $: rail = buildRail({
    installed: plugins.length,
    sources: forgeSources(sources).length,
    needsAttention: countNeedsAttention(plugins),
  });

  function fail(message: string) {
    errorMessage = message;
  }

  async function load() {
    busy = true;
    errorMessage = '';
    try {
      const data = await loadPluginsScreen();
      plugins = data.plugins;
      sources = data.sources;
      void Promise.all(sources.map((s) => refreshSource(s, false)));
    } finally {
      busy = false;
    }
  }

  async function refreshSource(source: PluginSourceDTO, force: boolean) {
    if (source.kind !== 'forge' || !source.available) return;
    loadingSources = { ...loadingSources, [source.id]: true };
    try {
      const found = await fetchSourceCatalog(source, force);
      pluginsBySource = { ...pluginsBySource, [source.id]: found };
      const { [source.id]: _dropped, ...rest } = errorsBySource;
      errorsBySource = rest;
    } catch (e) {
      errorsBySource = {
        ...errorsBySource,
        [source.id]: e instanceof Error ? e.message : 'Failed to fetch plugins',
      };
    } finally {
      const { [source.id]: _done, ...rest } = loadingSources;
      loadingSources = rest;
    }
  }

  async function withPluginBusy(id: string, work: () => Promise<void>) {
    busyPluginId = id;
    try {
      await work();
    } catch (e) {
      fail(e instanceof Error ? e.message : String(e));
    } finally {
      busyPluginId = '';
    }
  }

  const toggleEnabled = (plugin: PluginInfo, enabled: boolean) =>
    withPluginBusy(plugin.id, async () => {
      await setPluginEnabled(plugin.id, enabled);
      plugins = await listPlugins();
    });

  const ping = (plugin: PluginInfo) =>
    withPluginBusy(plugin.id, () => pingPlugin(plugin.id));

  function closeUninstall() {
    uninstallTarget = null;
    uninstallRemoveData = false;
  }

  async function confirmUninstall() {
    const target = uninstallTarget;
    const removeData = uninstallRemoveData;
    closeUninstall();
    if (!target) return;
    await withPluginBusy(target.id, async () => {
      plugins = await removePlugin(target.id, removeData);
      await Promise.all(sources.map((s) => refreshSource(s, true)));
    });
  }

  async function pickAndInstall(pick: () => Promise<string>) {
    const path = await pick();
    if (path) await installFlow.fromPath(path);
  }

  async function addSource(url: string, trusted: boolean) {
    addSourceBusy = true;
    try {
      await addGitHubRepository(url, trusted);
      addSourceOpen = false;
      const before = new Set(sources.map((s) => s.id));
      sources = await listPluginSources();
      // The backend normalises the URL it stores, so the typed string is not a key. Whichever
      // source is new is the one to fetch - matching on the raw input would miss a repository
      // registered as https://github.com/o/r after the user typed "o/r".
      const added = sources.filter((s) => !before.has(s.id));
      await Promise.all(added.map((s) => refreshSource(s, true)));
    } catch (e) {
      fail(e instanceof Error ? e.message : 'Failed to add repository');
    } finally {
      addSourceBusy = false;
    }
  }

  async function mutateSource(work: () => Promise<void>) {
    busy = true;
    try {
      await work();
      sources = await listPluginSources();
    } catch (e) {
      fail(e instanceof Error ? e.message : String(e));
    } finally {
      busy = false;
    }
  }
</script>

<Modal title="Plugins" {show} contentClass="plugins-modal" on:close={() => (show = false)}>
  <PluginsToolbar
    bind:query
    searchable={section === 'installed' || section === 'browse'}
    on:installFolder={() => pickAndInstall(selectPluginSourceDir)}
    on:installBundle={() => pickAndInstall(selectPluginBundleFile)}
    on:refreshAll={() => void Promise.all(sources.map((s) => refreshSource(s, true)))}
  />

  {#if errorMessage}
    <div class="screen-error">
      {errorMessage}
      <button class="ghost dismiss" on:click={() => (errorMessage = '')}>Dismiss</button>
    </div>
  {/if}

  <div class="plugins-body">
    <nav class="rail">
      {#each rail as item (item.id)}
        <button
          class="rail-item"
          class:active={section === item.id}
          aria-current={section === item.id ? 'page' : undefined}
          on:click={() => (section = item.id)}
        >
          <svelte:component this={RAIL_ICONS[item.id]} size={14} />
          <span class="rail-label">{item.label}</span>
          {#if item.alert}
            <span class="rail-alert" title="Something needs attention"></span>
          {/if}
          {#if item.count !== undefined}<span class="rail-count">{item.count}</span>{/if}
        </button>
      {/each}
    </nav>

    <div class="section-pane">
      {#if busy && plugins.length === 0 && sources.length === 0}
        <p class="loading">Loading…</p>
      {:else if section === 'installed'}
        <InstalledSection
          {plugins}
          {query}
          {busyPluginId}
          on:toggle={(e) => toggleEnabled(e.detail.plugin, e.detail.enabled)}
          on:ping={(e) => ping(e.detail.plugin)}
          on:uninstall={(e) => (uninstallTarget = e.detail.plugin)}
        />
      {:else if section === 'browse'}
        <BrowseSection
          {sources}
          {pluginsBySource}
          {errorsBySource}
          {loadingSources}
          {selectedTags}
          {query}
          {busyPluginId}
          on:refreshSource={(e) => refreshSource(e.detail.source, true)}
          on:selectTag={(e) => (selectedTags = { ...selectedTags, [e.detail.pluginId]: e.detail.tag })}
          on:details={(e) => {
            detailsPlugin = e.detail.plugin;
            detailsSource = e.detail.source;
          }}
          on:install={(e) => installFlow.fromSource(e.detail.source, e.detail.plugin, e.detail.releaseTag)}
          on:goToSources={() => (section = 'sources')}
        />
      {:else if section === 'sources'}
        <SourcesSection
          {sources}
          {loadingSources}
          {busy}
          on:addSource={() => (addSourceOpen = true)}
          on:refreshSource={(e) => refreshSource(e.detail.source, true)}
          on:setTrust={(e) =>
            mutateSource(() => setGitHubRepositoryTrust(e.detail.source.id, e.detail.trusted))}
          on:removeSource={(e) => mutateSource(() => removeGitHubRepository(e.detail.source.id))}
        />
      {:else}
        <MarketplaceSection />
      {/if}
    </div>
  </div>
</Modal>

<InstallFlow
  bind:this={installFlow}
  on:error={(e) => fail(e.detail.message)}
  on:installed={(e) => {
    plugins = e.detail.plugins;
    void Promise.all(sources.map((s) => refreshSource(s, true)));
  }}
/>

<PluginDetails
  show={detailsPlugin !== null}
  plugin={detailsPlugin}
  source={detailsSource}
  on:close={() => (detailsPlugin = null)}
/>

<AddSourceDialog
  show={addSourceOpen}
  busy={addSourceBusy}
  on:cancel={() => (addSourceOpen = false)}
  on:add={(e) => addSource(e.detail.url, e.detail.trusted)}
/>

<!-- The "delete data" choice rides in ConfirmDialog's body slot rather than its requireCheckbox
     prop: that prop is a gate on the confirm button (tick to proceed), and its confirm event
     carries no detail, so an optional answer cannot travel that way. -->
<ConfirmDialog
  show={uninstallTarget !== null}
  critical={true}
  title="Uninstall plugin"
  message={`Remove ${uninstallTarget?.name ?? ''}?`}
  confirmLabel="Uninstall"
  on:cancel={closeUninstall}
  on:confirm={confirmUninstall}
>
  <label slot="body" class="remove-data">
    <input type="checkbox" bind:checked={uninstallRemoveData} />
    Also delete this plugin's stored data
  </label>
</ConfirmDialog>

<style>
  .screen-error {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    margin-bottom: 10px;
    padding: 7px 10px;
    border-radius: 5px;
    background: rgba(197, 80, 80, 0.14);
    color: var(--danger);
    font-size: 11.5px;
  }

  .dismiss {
    font-size: 11px;
    padding: 2px 6px;
  }

  /* Fills whatever the dialog's fixed height leaves after the toolbar, so switching sections never
     changes the size of the window. min-height: 0 is what lets the pane inside scroll instead of
     stretching this row past the dialog. */
  .plugins-body {
    display: flex;
    gap: 18px;
    flex: 1;
    min-height: 0;
  }

  .rail {
    display: flex;
    flex-direction: column;
    gap: 1px;
    width: 168px;
    flex-shrink: 0;
    padding-right: 14px;
    border-right: 1px solid var(--border-color);
  }

  .rail-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 9px;
    border: none;
    border-left: 2px solid transparent;
    border-radius: 4px;
    background: transparent;
    color: var(--text-secondary);
    font-size: 12px;
    text-align: left;
    cursor: pointer;
  }

  .rail-item:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }

  .rail-item.active {
    background: var(--bg-active);
    border-left-color: var(--accent);
    color: var(--text-bright);
    font-weight: 600;
  }

  .rail-item:focus-visible {
    outline: 1px solid var(--border-focus);
    outline-offset: -1px;
  }

  .rail-label {
    flex: 1;
    min-width: 0;
  }

  /* A dot, not a number: "4" and "4, one of which is broken" must not look alike. */
  .rail-alert {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    background: var(--warning);
    flex-shrink: 0;
  }

  .rail-count {
    font-family: var(--font-mono);
    font-size: 10px;
    padding: 0 5px;
    border-radius: 3px;
    background: var(--bg-input);
    color: var(--text-secondary);
  }

  .section-pane {
    flex: 1;
    min-width: 0;
    min-height: 0;
    overflow-y: auto;
    padding-right: 4px;
  }

  .loading {
    margin: 0;
    font-size: 12px;
    color: var(--text-secondary);
  }

  .remove-data {
    display: flex;
    align-items: center;
    gap: 7px;
    margin-top: 8px;
    font-size: 12px;
    cursor: pointer;
  }
</style>
