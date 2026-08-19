<script lang="ts">
  // The Plugins screen: rail on the left, one section on the right, and the dialogs that overlay
  // both. It owns the screen's data and routes events; every rendering decision belongs to a
  // section, and every multi-RPC sequence to actions/pluginsActions.
  import { MoreHorizontal, Search } from 'lucide-svelte';
  import Modal from '../Modal.svelte';
  import ConfirmDialog from '../ConfirmDialog.svelte';
  import InstalledSection from './InstalledSection.svelte';
  import BrowseSection from './BrowseSection.svelte';
  import SourcesSection from './SourcesSection.svelte';
  import SecuritySection from './SecuritySection.svelte';
  import PluginDetails from './PluginDetails.svelte';
  import AddSourceDialog from './AddSourceDialog.svelte';
  import InstallFlow from './InstallFlow.svelte';
  import { buildRail, type PluginsSectionId } from './pluginsView';
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
  let menuOpen = false;
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

  let loadedOnce = false;
  $: if (show && !loadedOnce) {
    loadedOnce = true;
    void load();
  }
  $: if (!show) loadedOnce = false;

  $: rail = buildRail(plugins.length, sources.length);

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

  async function confirmUninstall(removeData: boolean) {
    const target = uninstallTarget;
    uninstallTarget = null;
    if (!target) return;
    await withPluginBusy(target.id, async () => {
      plugins = await removePlugin(target.id, removeData);
      await Promise.all(sources.map((s) => refreshSource(s, true)));
    });
  }

  async function pickAndInstall(pick: () => Promise<string>) {
    menuOpen = false;
    const path = await pick();
    if (path) await installFlow.fromPath(path);
  }

  async function addSource(url: string, trusted: boolean) {
    addSourceBusy = true;
    try {
      await addGitHubRepository(url, trusted);
      addSourceOpen = false;
      sources = await listPluginSources();
      const added = sources.find((s) => s.id.endsWith(url.replace(/^https?:\/\//, '')));
      if (added) await refreshSource(added, true);
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
  <div class="plugins-toolbar">
    <div class="search-box">
      <Search size={13} />
      <input type="text" placeholder="Search plugins…" bind:value={query} />
    </div>
    <div class="menu-wrap">
      <button class="ghost icon-btn" title="More actions" on:click={() => (menuOpen = !menuOpen)}>
        <MoreHorizontal size={15} />
      </button>
      {#if menuOpen}
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <div class="menu" on:mouseleave={() => (menuOpen = false)}>
          <button class="menu-item" on:click={() => pickAndInstall(selectPluginSourceDir)}>
            Install from folder…
          </button>
          <button class="menu-item" on:click={() => pickAndInstall(selectPluginBundleFile)}>
            Install from bundle…
          </button>
          <div class="menu-sep"></div>
          <button
            class="menu-item"
            on:click={() => {
              menuOpen = false;
              void Promise.all(sources.map((s) => refreshSource(s, true)));
            }}
          >
            Refresh all sources
          </button>
        </div>
      {/if}
    </div>
  </div>

  {#if errorMessage}
    <div class="screen-error">
      {errorMessage}
      <button class="ghost dismiss" on:click={() => (errorMessage = '')}>Dismiss</button>
    </div>
  {/if}

  <div class="plugins-body">
    <nav class="rail">
      {#each rail as item (item.id)}
        <button class="rail-item" class:active={section === item.id} on:click={() => (section = item.id)}>
          <span>{item.label}</span>
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
        <SecuritySection onError={fail} />
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

<ConfirmDialog
  show={uninstallTarget !== null}
  critical={true}
  title="Uninstall plugin"
  message={`Remove ${uninstallTarget?.name ?? ''}?`}
  requireCheckbox={false}
  checkboxLabel="Also delete this plugin's stored data"
  confirmLabel="Uninstall"
  on:cancel={() => (uninstallTarget = null)}
  on:confirm={(e) => confirmUninstall(e.detail?.checked === true)}
/>

<style>
  .plugins-toolbar {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 10px;
  }

  .search-box {
    display: flex;
    align-items: center;
    gap: 6px;
    flex: 1;
    padding: 4px 8px;
    border: 1px solid var(--border);
    border-radius: 5px;
    background: var(--bg-secondary);
    color: var(--text-secondary);
  }

  .search-box input {
    flex: 1;
    border: none;
    background: transparent;
    outline: none;
    font-size: 12px;
    color: var(--text);
  }

  .menu-wrap {
    position: relative;
  }

  .menu {
    position: absolute;
    right: 0;
    top: calc(100% + 4px);
    z-index: 10;
    min-width: 190px;
    padding: 4px;
    border: 1px solid var(--border);
    border-radius: 6px;
    background: var(--bg-elevated, var(--bg-secondary));
    box-shadow: 0 6px 20px rgba(0, 0, 0, 0.35);
  }

  .menu-item {
    display: block;
    width: 100%;
    padding: 6px 9px;
    border: none;
    border-radius: 4px;
    background: transparent;
    color: inherit;
    font-size: 12px;
    text-align: left;
    cursor: pointer;
  }

  .menu-item:hover {
    background: var(--bg-hover, rgba(255, 255, 255, 0.06));
  }

  .menu-sep {
    height: 1px;
    margin: 4px 2px;
    background: var(--border);
  }

  .screen-error {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 10px;
    margin-bottom: 10px;
    padding: 7px 10px;
    border-radius: 5px;
    background: rgba(248, 81, 73, 0.12);
    color: var(--danger, #f85149);
    font-size: 11.5px;
  }

  .dismiss {
    font-size: 11px;
    padding: 2px 6px;
  }

  .plugins-body {
    display: flex;
    gap: 14px;
    min-height: 52vh;
    max-height: 62vh;
  }

  .rail {
    display: flex;
    flex-direction: column;
    gap: 2px;
    width: 150px;
    flex-shrink: 0;
  }

  .rail-item {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 6px 9px;
    border: none;
    border-radius: 5px;
    background: transparent;
    color: var(--text-secondary);
    font-size: 12px;
    text-align: left;
    cursor: pointer;
  }

  .rail-item:hover {
    background: var(--bg-hover, rgba(255, 255, 255, 0.05));
  }

  .rail-item.active {
    background: var(--bg-secondary);
    color: var(--text);
    font-weight: 600;
  }

  .rail-count {
    font-size: 10.5px;
    padding: 0 6px;
    border-radius: 8px;
    background: var(--bg-elevated, rgba(255, 255, 255, 0.08));
  }

  .section-pane {
    flex: 1;
    min-width: 0;
    overflow-y: auto;
    padding-right: 4px;
  }

  .loading {
    margin: 0;
    font-size: 12px;
    color: var(--text-secondary);
  }

  .icon-btn {
    display: inline-flex;
    align-items: center;
    padding: 4px;
  }
</style>
