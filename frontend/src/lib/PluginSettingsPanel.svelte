<script lang="ts">

  import { onMount } from 'svelte';

  import {

    listPlugins,

    selectPluginSourceDir,

    selectPluginBundleFile,

    previewPluginInstall,

    pingPlugin,
    setPluginEnabled,

    getPluginSettings,

    savePluginSettings,

    generatePluginPublisherKeyPair,

    type PluginInfo,

    type PluginInstallPreview,

    type PluginSettings,

  } from '../api/plugins';

  import {

    listGitHubRepositories,

    addGitHubRepository,

    removeGitHubRepository,

    setGitHubRepositoryTrust,

    fetchGitHubPlugins,

    previewGitHubPluginInstall,

    type GitHubRepository,

    type GitHubPluginMetadata,

    type GitHubPluginPreview,

    type GitHubReleaseSummary,

  } from '../api/githubPlugins';

  import { installPlugin, installGitHubPlugin, uninstallGitHubPlugin } from '../actions/protocolActions';

  import ConfirmDialog from './ConfirmDialog.svelte';
  import Modal from './Modal.svelte';
  import GitHubReadmePanel from './GitHubReadmePanel.svelte';
  import { formatPublishedDate } from './githubReadme';
  import {
    defaultReleaseTagForPlugin,
    formatInstalledVersion,
    githubInstallPreviewLines,
    githubPluginStatusLabel,
  } from './pluginDisplay';
  import { createSingleFlightRunner } from './repoFetchSingleFlight';
  import { Puzzle, ShieldAlert, BadgeCheck, FileArchive, FolderOpen, Github, RefreshCw } from 'lucide-svelte';

  export let showAdvanced = false;



  let plugins: PluginInfo[] = [];

  let loading = true;

  let installPreview: PluginInstallPreview | null = null;

  let pendingSourcePath = '';

  let installConfirmOpen = false;

  let installBusy = false;

  let grantSecretAccess = false;

  let grantAuthProviderAccess = false;

  let grantTunnelProviderAccess = false;

  let grantMultiSessionAccess = false;

  let grantArbitraryNetworkAccess = false;
  let grantExecAccess = false;

  let errorMessage = '';

  let pluginSettings: PluginSettings = { trustedPublisherKeys: [], requireSignedPlugins: false };

  let newTrustedKey = '';

  let settingsBusy = false;

  let activeTab: 'installed' | 'github' = 'installed';

  let repositories: GitHubRepository[] = [];

  let repoPlugins: Record<string, GitHubPluginMetadata[]> = {};

  let reposLoading = false;

  let repoFetchErrors: Record<string, string> = {};

  let refreshingRepoUrl = '';

  const runRepoPluginsFetch = createSingleFlightRunner<void>();

  let githubReposLoadPromise: Promise<void> | null = null;

  let githubPreviewBusy = false;

  let previewingPluginId = '';

  let addRepoDialogOpen = false;

  let addRepoUntrustedConfirmOpen = false;

  let addRepoBusy = false;

  let newRepoURL = '';

  let githubInstallPreview: GitHubPluginPreview | null = null;

  let githubInstallConfirmOpen = false;

  let githubInstallTrustConfirmed = false;

  let githubGrantSecretAccess = false;

  let githubGrantAuthProviderAccess = false;

  let githubGrantTunnelProviderAccess = false;

  let githubGrantMultiSession = false;

  let githubGrantArbitraryNetwork = false;
  let githubGrantExecAccess = false;

  let pendingGitHubRepoURL = '';

  let pendingGitHubReleaseTag = '';

  let githubInstallBusy = false;

  let pluginDetailsOpen = false;

  let selectedGitHubPlugin: GitHubPluginMetadata | null = null;

  let selectedDetailsRepoURL = '';

  let selectedReleaseByPlugin: Record<string, string> = {};

  let uninstallConfirmOpen = false;

  let removePluginData = false;

  let pendingUninstallPlugin: GitHubPluginMetadata | null = null;

  function isGitHubRepositoryURL(value: string): boolean {
    const trimmed = value.trim();
    if (!trimmed) return false;

    let url = trimmed.replace(/\/+$/, '');
    if (!/^https?:\/\//i.test(url)) {
      url = `https://github.com/${url.replace(/^github\.com\/?/i, '')}`;
    }

    try {
      const parsed = new URL(url);
      if (parsed.protocol !== 'https:') return false;
      if (parsed.hostname !== 'github.com') return false;

      const parts = parsed.pathname.replace(/^\/+|\/+$/g, '').split('/').filter(Boolean);
      if (parts.length < 2) return false;
      if (!/^[\w.-]+$/.test(parts[0]) || !/^[\w.-]+$/.test(parts[1])) return false;
      return true;
    } catch {
      return false;
    }
  }

  $: newRepoURLValid = isGitHubRepositoryURL(newRepoURL);
  $: showNewRepoURLError = newRepoURL.trim().length > 0 && !newRepoURLValid;

  function defaultReleaseTag(plugin: GitHubPluginMetadata): string {
    return defaultReleaseTagForPlugin(plugin);
  }

  function getSelectedReleaseTag(plugin: GitHubPluginMetadata): string {
    return selectedReleaseByPlugin[plugin.id] ?? defaultReleaseTag(plugin);
  }

  function getSelectedRelease(plugin: GitHubPluginMetadata): GitHubReleaseSummary | null {
    const tag = getSelectedReleaseTag(plugin);
    return plugin.availableReleases?.find((release) => release.tag === tag) ?? plugin.availableReleases?.[0] ?? null;
  }

  function releaseOptionLabel(release: GitHubReleaseSummary): string {
    return release.prerelease ? `${release.tag} (pre-release)` : release.tag;
  }

  function syncSelectedReleaseTags(plugins: GitHubPluginMetadata[]) {
    const next = { ...selectedReleaseByPlugin };
    for (const plugin of plugins) {
      next[plugin.id] = defaultReleaseTag(plugin);
    }
    selectedReleaseByPlugin = next;
  }

  let previousActiveTab: 'installed' | 'github' = activeTab;
  let repoFetchInFlight: Record<string, boolean> = {};

  $: if (activeTab !== previousActiveTab) {
    previousActiveTab = activeTab;
    if (activeTab === 'github') {
      void backgroundRefreshGitHub();
    } else {
      void refreshPlugins();
    }
  }

  async function backgroundRefreshGitHub() {
    if (githubReposLoadPromise || repositories.length === 0) return;
    await Promise.all(
      repositories.map((repo) => {
        if (repoPlugins[repo.url]?.length) {
          return Promise.resolve();
        }
        return refreshRepoPlugins(repo.url, false);
      }),
    );
  }

  function setSelectedReleaseTag(pluginId: string, tag: string) {
    selectedReleaseByPlugin = { ...selectedReleaseByPlugin, [pluginId]: tag };
  }



  onMount(() => {

    refreshPlugins();

    loadPluginSettings();

    loadGitHubRepositories();

  });



  async function loadPluginSettings() {

    try {

      pluginSettings = await getPluginSettings();

    } catch (e) {

      errorMessage = e instanceof Error ? e.message : 'Failed to load plugin settings';

    }

  }



  async function refreshPlugins() {

    loading = true;

    errorMessage = '';

    try {

      plugins = await listPlugins();

    } catch (e) {

      errorMessage = e instanceof Error ? e.message : 'Failed to load plugins';

    } finally {

      loading = false;

    }

  }



  async function beginInstall(sourcePath: string) {

    if (!sourcePath) return;

    errorMessage = '';

    try {

      pendingSourcePath = sourcePath;

      installPreview = await previewPluginInstall(sourcePath);

      grantSecretAccess = false;
      grantAuthProviderAccess = false;
      grantTunnelProviderAccess = false;
      grantMultiSessionAccess = false;
      grantArbitraryNetworkAccess = false;
      grantExecAccess = false;

      installConfirmOpen = true;

    } catch (e) {

      errorMessage = e instanceof Error ? e.message : 'Failed to preview plugin';

    }

  }



  async function startInstallFromFolder() {

    const dir = await selectPluginSourceDir();

    await beginInstall(dir);

  }



  async function startInstallFromBundle() {

    const bundle = await selectPluginBundleFile();

    await beginInstall(bundle);

  }



  async function confirmInstall() {

    if (!pendingSourcePath) return;

    installBusy = true;

    errorMessage = '';

    try {

      await installPlugin(
        pendingSourcePath,
        grantSecretAccess,
        grantAuthProviderAccess,
        grantTunnelProviderAccess,
        grantMultiSessionAccess,
        grantArbitraryNetworkAccess,
        grantExecAccess,
      );

      installConfirmOpen = false;

      installPreview = null;

      pendingSourcePath = '';

      await refreshPlugins();

    } catch (e) {

      errorMessage = e instanceof Error ? e.message : 'Install failed';

    } finally {

      installBusy = false;

    }

  }



  async function handlePing(pluginId: string) {

    errorMessage = '';

    try {

      await pingPlugin(pluginId);

      await refreshPlugins();

    } catch (e) {

      errorMessage = e instanceof Error ? e.message : 'Ping failed';

    }

  }



  async function toggleEnabled(plugin: PluginInfo, event: Event) {
    const enabled = (event.target as HTMLInputElement).checked;
    errorMessage = '';
    try {
      await setPluginEnabled(plugin.id, enabled);
      await refreshPlugins();
    } catch (e) {
      errorMessage = e instanceof Error ? e.message : 'Failed to update plugin';
    }
  }



  function cancelInstall() {

    installConfirmOpen = false;

    installPreview = null;

    pendingSourcePath = '';

  }



  async function saveTrustSettings() {

    settingsBusy = true;

    errorMessage = '';

    try {

      await savePluginSettings(pluginSettings);

    } catch (e) {

      errorMessage = e instanceof Error ? e.message : 'Failed to save plugin settings';

    } finally {

      settingsBusy = false;

    }

  }



  async function addTrustedKey() {

    const key = newTrustedKey.trim();

    if (!key) return;

    if (pluginSettings.trustedPublisherKeys.includes(key)) {

      newTrustedKey = '';

      return;

    }

    pluginSettings = {

      ...pluginSettings,

      trustedPublisherKeys: [...pluginSettings.trustedPublisherKeys, key],

    };

    newTrustedKey = '';

    await saveTrustSettings();

  }



  async function removeTrustedKey(key: string) {

    pluginSettings = {

      ...pluginSettings,

      trustedPublisherKeys: pluginSettings.trustedPublisherKeys.filter((k) => k !== key),

    };

    await saveTrustSettings();

  }



  async function generatePublisherKeys() {

    errorMessage = '';

    try {

      const pair = await generatePluginPublisherKeyPair();

      if (!pair.publicKey) return;

      newTrustedKey = pair.publicKey;

    } catch (e) {

      errorMessage = e instanceof Error ? e.message : 'Key generation failed';

    }

  }

  async function loadGitHubRepositories(forceRefresh = false) {
    if (githubReposLoadPromise) {
      return githubReposLoadPromise;
    }

    reposLoading = true;
    githubReposLoadPromise = (async () => {
      try {
        repositories = await listGitHubRepositories();
        await Promise.all(repositories.map((repo) => refreshRepoPlugins(repo.url, forceRefresh)));
      } catch (e) {
        errorMessage = e instanceof Error ? e.message : 'Failed to load GitHub repositories';
      } finally {
        reposLoading = false;
        githubReposLoadPromise = null;
      }
    })();

    return githubReposLoadPromise;
  }

  async function refreshRepoPlugins(repoURL: string, forceRefresh = false) {
    return runRepoPluginsFetch(repoURL, async () => {
      repoFetchInFlight = { ...repoFetchInFlight, [repoURL]: true };
      if (forceRefresh) {
        refreshingRepoUrl = repoURL;
      }

      try {
        const nextErrors = { ...repoFetchErrors };
        delete nextErrors[repoURL];
        repoFetchErrors = nextErrors;

        const result = await fetchGitHubPlugins(repoURL, forceRefresh);
        if (result.plugins?.length) {
          repoPlugins = { ...repoPlugins, [repoURL]: result.plugins };
          syncSelectedReleaseTags(result.plugins);
        } else {
          const cleared = { ...repoPlugins };
          delete cleared[repoURL];
          repoPlugins = cleared;
        }
      } catch (e) {
        repoFetchErrors = {
          ...repoFetchErrors,
          [repoURL]: e instanceof Error ? e.message : 'Failed to fetch plugins',
        };
      } finally {
        const nextInFlight = { ...repoFetchInFlight };
        delete nextInFlight[repoURL];
        repoFetchInFlight = nextInFlight;
        if (refreshingRepoUrl === repoURL) {
          refreshingRepoUrl = '';
        }
      }
    });
  }

  function showAddRepoDialog() {
    newRepoURL = '';
    addRepoUntrustedConfirmOpen = false;
    addRepoBusy = false;
    addRepoDialogOpen = true;
  }

  function closeAddRepoDialog() {
    addRepoDialogOpen = false;
    addRepoUntrustedConfirmOpen = false;
    addRepoBusy = false;
  }

  function proceedAddRepo() {
    if (!newRepoURLValid || addRepoBusy) return;
    errorMessage = '';
    addRepoUntrustedConfirmOpen = true;
  }

  function cancelAddRepoUntrustedConfirm() {
    addRepoUntrustedConfirmOpen = false;
  }

  async function confirmAddRepo() {
    if (!newRepoURLValid || addRepoBusy) return;
    addRepoBusy = true;
    errorMessage = '';
    try {
      await addGitHubRepository(newRepoURL.trim(), false);
      closeAddRepoDialog();
      await loadGitHubRepositories(true);
    } catch (e) {
      errorMessage = e instanceof Error ? e.message : 'Failed to add repository';
    } finally {
      addRepoBusy = false;
    }
  }

  async function toggleRepoTrust(repo: GitHubRepository) {
    errorMessage = '';
    try {
      await setGitHubRepositoryTrust(repo.url, !repo.trusted);
      await loadGitHubRepositories(false);
    } catch (e) {
      errorMessage = e instanceof Error ? e.message : 'Failed to update trust';
    }
  }

  async function removeRepo(repoURL: string) {
    errorMessage = '';
    try {
      await removeGitHubRepository(repoURL);
      const next = { ...repoPlugins };
      delete next[repoURL];
      repoPlugins = next;
      await loadGitHubRepositories(true);
    } catch (e) {
      errorMessage = e instanceof Error ? e.message : 'Failed to remove repository';
    }
  }

  function showPluginDetails(plugin: GitHubPluginMetadata, repoURL: string) {
    selectedGitHubPlugin = plugin;
    selectedDetailsRepoURL = repoURL;
    pluginDetailsOpen = true;
  }

  function closePluginDetails() {
    pluginDetailsOpen = false;
    selectedGitHubPlugin = null;
    selectedDetailsRepoURL = '';
  }

  async function showGitHubInstallConfirm(repoURL: string, plugin: GitHubPluginMetadata) {
    if (githubPreviewBusy || githubInstallBusy) return;
    errorMessage = '';
    pendingGitHubRepoURL = repoURL;
    pendingGitHubReleaseTag = getSelectedReleaseTag(plugin);
    githubPreviewBusy = true;
    previewingPluginId = plugin.id;
    try {
      githubInstallPreview = await previewGitHubPluginInstall(repoURL, pendingGitHubReleaseTag);
      githubInstallTrustConfirmed = false;
      githubGrantSecretAccess = false;
      githubGrantAuthProviderAccess = false;
      githubGrantTunnelProviderAccess = false;
      githubGrantMultiSession = false;
      githubGrantArbitraryNetwork = false;
      githubGrantExecAccess = false;
      githubInstallConfirmOpen = true;
      selectedGitHubPlugin = plugin;
    } catch (e) {
      errorMessage = e instanceof Error ? e.message : 'Failed to preview plugin';
    } finally {
      githubPreviewBusy = false;
      previewingPluginId = '';
    }
  }

  function closeGitHubInstallConfirm() {
    githubInstallConfirmOpen = false;
    githubInstallPreview = null;
    pendingGitHubRepoURL = '';
    pendingGitHubReleaseTag = '';
  }

  async function confirmGitHubInstall() {
    if (!pendingGitHubRepoURL || !githubInstallTrustConfirmed) return;
    githubInstallBusy = true;
    errorMessage = '';
    try {
      await installGitHubPlugin(
        pendingGitHubRepoURL,
        pendingGitHubReleaseTag,
        githubGrantSecretAccess,
        githubGrantAuthProviderAccess,
        githubGrantTunnelProviderAccess,
        githubGrantMultiSession,
        githubGrantArbitraryNetwork,
        githubGrantExecAccess,
      );
      closeGitHubInstallConfirm();
      if (selectedGitHubPlugin?.id) {
        const next = { ...selectedReleaseByPlugin };
        delete next[selectedGitHubPlugin.id];
        selectedReleaseByPlugin = next;
      }
      await refreshPlugins();
      await loadGitHubRepositories(true);
    } catch (e) {
      errorMessage = e instanceof Error ? e.message : 'Install failed';
    } finally {
      githubInstallBusy = false;
    }
  }

  function showUninstallConfirm(plugin: GitHubPluginMetadata) {
    pendingUninstallPlugin = plugin;
    removePluginData = false;
    uninstallConfirmOpen = true;
  }

  function closeUninstallConfirm() {
    uninstallConfirmOpen = false;
    pendingUninstallPlugin = null;
  }

  async function confirmUninstall() {
    if (!pendingUninstallPlugin) return;
    errorMessage = '';
    try {
      await uninstallGitHubPlugin(pendingUninstallPlugin.id, removePluginData);
      closeUninstallConfirm();
      await refreshPlugins();
      await loadGitHubRepositories(true);
    } catch (e) {
      errorMessage = e instanceof Error ? e.message : 'Uninstall failed';
    }
  }

  $: githubInstallPreviewMessage = githubInstallPreview
    ? githubInstallPreviewLines(
        githubInstallPreview.name,
        pendingGitHubReleaseTag || githubInstallPreview.releaseTag || githubInstallPreview.latestRelease,
        githubInstallPreview.version,
      ).join('\n')
    : '';

  function isRepoPluginsLoading(repoURL: string): boolean {
    return repoFetchInFlight[repoURL] === true;
  }



  $: installMessage = installPreview

    ? [

        `${installPreview.name} v${installPreview.version}`,

        installPreview.id,

        installPreview.signatureVerified ? 'Signature verified against trusted publishers.' : '',

        installPreview.untrustedSignatureWarning ? 'Signature present but not trusted.' : '',

        installPreview.unsignedWarning ? 'This plugin is not signed or not trusted. Install anyway?' : '',

        installPreview.checksumPresent ? 'Bundle checksums validated.' : '',

        installPreview.requiresSecretAccess ? 'This plugin will have access to connection passwords.' : '',

        installPreview.arbitraryNetworkWarning ? 'This plugin can open TCP connections to arbitrary hosts on the internet.' : '',

        installPreview.execAccessWarning ? 'This plugin can run commands on the hosts you connect to, over your authenticated session.' : '',

        'Permissions:',

        ...installPreview.permissions.map((p) => `• ${p}`),

      ].filter(Boolean).join('\n')

    : '';

</script>



<div class="plugin-settings">

  <div class="tab-row">
    <button type="button" class="tab-btn" class:active={activeTab === 'installed'} on:click={() => activeTab = 'installed'}>
      Installed
    </button>
    <button type="button" class="tab-btn" class:active={activeTab === 'github'} on:click={() => activeTab = 'github'}>
      <Github size={14} /> GitHub
    </button>
  </div>

  {#if activeTab === 'installed'}

  <div class="section-header">

    <h3>Plugins</h3>

    {#if showAdvanced}
    <div class="install-actions">

      <button type="button" class="btn-secondary" on:click={startInstallFromFolder}>

        <FolderOpen size={14} />

        Install folder…

      </button>

      <button type="button" class="btn-secondary" on:click={startInstallFromBundle}>

        <FileArchive size={14} />

        Install bundle…

      </button>

    </div>
    {/if}

  </div>



  <p class="section-desc">

    Manage out-of-process plugins. Install from a folder or a signed `.xqsp` bundle.

  </p>

  {#if !showAdvanced}
    <p class="section-desc">Enable <strong>Advanced</strong> in the footer to install from a folder or bundle and manage publisher trust.</p>
  {/if}



  {#if showAdvanced}
  <div class="trust-panel">

    <h4>Trust policy</h4>

    <label class="checkbox-row">

      <input type="checkbox" bind:checked={pluginSettings.requireSignedPlugins} on:change={saveTrustSettings} />

      Require signed plugins from trusted publishers

    </label>

    <div class="trusted-keys">

      <label for="trusted-key">Trusted publisher keys (base64 Ed25519 public keys)</label>

      <div class="key-row">

        <input id="trusted-key" class="key-input" bind:value={newTrustedKey} placeholder="Paste public key…" />

        <button type="button" class="btn-secondary" disabled={settingsBusy} on:click={addTrustedKey}>Add</button>

        <button type="button" class="btn-secondary" disabled={settingsBusy} on:click={generatePublisherKeys}>Generate pair</button>

      </div>

      {#if pluginSettings.trustedPublisherKeys.length > 0}

        <ul class="key-list">

          {#each pluginSettings.trustedPublisherKeys as key (key)}

            <li>

              <code>{key.slice(0, 24)}…</code>

              <button type="button" class="link-btn" on:click={() => removeTrustedKey(key)}>Remove</button>

            </li>

          {/each}

        </ul>

      {/if}

    </div>

  </div>
  {/if}



  {#if errorMessage}

    <p class="error-text">{errorMessage}</p>

  {/if}



  {#if loading}

    <p class="muted">Loading plugins…</p>

  {:else if plugins.length === 0}

    <p class="muted">No plugins installed.</p>

  {:else}

    <ul class="plugin-list">

      {#each plugins as plugin (plugin.id)}

        <li class="plugin-row">

          <div class="plugin-main">

            <div class="plugin-title">

              <Puzzle size={14} />

              <strong>{plugin.name}</strong>

              <span class="version">v{plugin.version}</span>

              {#if plugin.signed}

                <span class="badge ok"><BadgeCheck size={12} /> Signed</span>

              {/if}

            </div>

            <div class="plugin-meta">{plugin.id} · {plugin.source} · {plugin.state}</div>

            {#if plugin.description}

              <div class="plugin-desc">{plugin.description}</div>

            {/if}

            {#if plugin.requiresSecretAccess}

              <div class="warn-line"><ShieldAlert size={12} /> Has secret access capability</div>

            {/if}

          </div>

          <div class="plugin-actions">
            <label class="checkbox-row enable-toggle">
              <input
                type="checkbox"
                checked={plugin.enabled !== false}
                on:change={(e) => toggleEnabled(plugin, e)}
              />
              Enabled
            </label>
            <button type="button" class="btn-secondary" on:click={() => handlePing(plugin.id)}>Ping</button>
          </div>

        </li>

      {/each}

    </ul>

  {/if}

  {:else}

  <div class="section-header">
    <h3>GitHub Repositories</h3>
    <button type="button" class="btn-secondary" on:click={showAddRepoDialog}>Add Repository</button>
  </div>

  <p class="section-desc">
    Discover and install plugins from public GitHub repositories with xqsp.json manifests.
  </p>

  {#if reposLoading}
    <p class="muted">Loading repositories…</p>
  {:else if repositories.length === 0}
    <p class="muted">No GitHub repositories added yet.</p>
  {:else}
    <ul class="repo-list">
      {#each repositories as repo (repo.url)}
        <li class="repo-item">
          <div class="repo-info">
            <strong>{repo.displayName}</strong>
            <span class="repo-url">{repo.url}</span>
            {#if repo.trusted}
              <span class="badge ok">Trusted</span>
            {:else}
              <span class="badge warn">Untrusted</span>
            {/if}
          </div>
          <div class="repo-actions">
            <button
              type="button"
              class="btn-secondary"
              disabled={refreshingRepoUrl === repo.url || isRepoPluginsLoading(repo.url)}
              on:click={() => refreshRepoPlugins(repo.url, true)}
            >
              <RefreshCw size={12} class={refreshingRepoUrl === repo.url ? 'spinning' : ''} />
              {refreshingRepoUrl === repo.url ? 'Refreshing…' : 'Refresh'}
            </button>
            <button type="button" class="btn-secondary" on:click={() => toggleRepoTrust(repo)}>
              {repo.trusted ? 'Untrust' : 'Trust'}
            </button>
            <button type="button" class="btn-danger" on:click={() => removeRepo(repo.url)}>Remove</button>
          </div>
          {#if isRepoPluginsLoading(repo.url)}
            <p class="muted">Loading plugins…</p>
          {:else if repoFetchErrors[repo.url]}
            <p class="warn-line">{repoFetchErrors[repo.url]}</p>
            <button type="button" class="btn-secondary" on:click={() => refreshRepoPlugins(repo.url, false)}>Retry</button>
          {:else if repoPlugins[repo.url]?.length}
            <div class="plugins-list">
              {#each repoPlugins[repo.url] as plugin (plugin.id)}
                {@const selectedRelease = getSelectedRelease(plugin)}
                {@const status = githubPluginStatusLabel(plugin)}
                <div class="plugin-card">
                  <div class="plugin-title">
                    <strong>{plugin.name}</strong>
                    {#if status.kind === 'installed'}
                      <span class="version">{status.text}</span>
                    {:else}
                      <span class="badge not-installed">{status.text}</span>
                      {#if plugin.latestRelease}
                        <span class="latest-release">Latest: {plugin.latestRelease}</span>
                      {/if}
                    {/if}
                    {#if !plugin.installed && selectedRelease?.prerelease}
                      <span class="badge warn">Pre-release</span>
                    {/if}
                  </div>
                  {#if plugin.description}
                    <p class="plugin-desc">{plugin.description}</p>
                  {/if}
                  {#if !(selectedRelease?.platformSupported ?? plugin.platformSupported)}
                    <p class="warn-line">Not compatible with your platform</p>
                  {/if}
                  {#if plugin.availableReleases?.length}
                    <div class="version-select-row">
                      <label for={`release-${plugin.id}`}>Version</label>
                      <select
                        id={`release-${plugin.id}`}
                        class="version-select"
                        value={getSelectedReleaseTag(plugin)}
                        on:change={(e) => setSelectedReleaseTag(plugin.id, e.currentTarget.value)}
                      >
                        {#each plugin.availableReleases as release (release.tag)}
                          <option value={release.tag}>{releaseOptionLabel(release)}</option>
                        {/each}
                      </select>
                    </div>
                  {/if}
                  <div class="plugin-actions-row">
                    <button type="button" class="btn-secondary" on:click={() => showPluginDetails(plugin, repo.url)}>Details</button>
                    {#if plugin.installed}
                      <button type="button" class="btn-danger" on:click={() => showUninstallConfirm(plugin)}>Uninstall</button>
                    {:else if selectedRelease?.platformSupported ?? plugin.platformSupported}
                      <button
                        type="button"
                        class="btn-secondary"
                        disabled={githubPreviewBusy && previewingPluginId === plugin.id}
                        on:click={() => showGitHubInstallConfirm(repo.url, plugin)}
                      >
                        {githubPreviewBusy && previewingPluginId === plugin.id ? 'Loading…' : 'Install'}
                      </button>
                    {/if}
                  </div>
                </div>
              {/each}
            </div>
          {:else}
            <p class="muted">No plugin metadata</p>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}

  {/if}

</div>



{#if installConfirmOpen && installPreview}
  <div class="dialog-overlay" role="presentation" on:click={cancelInstall} on:keydown={(e) => e.key === 'Escape' && cancelInstall()}>
    <div class="dialog" role="dialog" on:click|stopPropagation on:keydown|stopPropagation>
      <h4>Install {installPreview.name}?</h4>
      <pre class="install-preview">{installMessage}</pre>
      {#if installPreview.arbitraryNetworkWarning}
        <div class="warning-box">
          <strong>Network access warning</strong>
          <p>This plugin can open TCP connections to arbitrary hosts.</p>
        </div>
        <label class="checkbox-row">
          <input type="checkbox" bind:checked={grantArbitraryNetworkAccess} />
          I understand this plugin can connect to any host on the internet
        </label>
      {/if}
      {#if installPreview.requiresSecretAccess}
        <label class="checkbox-row">
          <input type="checkbox" bind:checked={grantSecretAccess} />
          I understand this plugin will have access to connection secrets
        </label>
      {/if}
      {#if installPreview.requiresAuthProviderAccess}
        <label class="checkbox-row">
          <input type="checkbox" bind:checked={grantAuthProviderAccess} />
          I understand this plugin can participate in SSH authentication
        </label>
      {/if}
      {#if installPreview.requiresTunnelProviderAccess}
        <label class="checkbox-row">
          <input type="checkbox" bind:checked={grantTunnelProviderAccess} />
          I understand this plugin can route dynamic port-forward connections
        </label>
      {/if}
      {#if installPreview.multiSessionWarning}
        <label class="checkbox-row">
          <input type="checkbox" bind:checked={grantMultiSessionAccess} />
          Allow multi-session access (if required)
        </label>
      {/if}
      {#if installPreview.execAccessWarning}
        <div class="warning-box">
          <strong>Command execution warning</strong>
          <p>This plugin can run commands on the hosts you connect to, using your authenticated session.</p>
        </div>
        <label class="checkbox-row">
          <input type="checkbox" bind:checked={grantExecAccess} />
          I understand this plugin can run commands on my sessions
        </label>
      {/if}
      <div class="dialog-actions">
        <button type="button" class="btn-secondary" on:click={cancelInstall}>Cancel</button>
        <button
          type="button"
          class="btn-secondary"
          disabled={
            installBusy
            || (installPreview.requiresSecretAccess && !grantSecretAccess)
            || (installPreview.requiresAuthProviderAccess && !grantAuthProviderAccess)
            || (installPreview.requiresTunnelProviderAccess && !grantTunnelProviderAccess)
            || (installPreview.arbitraryNetworkWarning && !grantArbitraryNetworkAccess)
            || (installPreview.multiSessionWarning && !grantMultiSessionAccess)
            || (installPreview.execAccessWarning && !grantExecAccess)
          }
          on:click={confirmInstall}
        >
          {installBusy ? 'Installing…' : 'Install'}
        </button>
      </div>
    </div>
  </div>
{/if}

{#if addRepoDialogOpen}
  <div class="dialog-overlay" role="presentation" on:click={closeAddRepoDialog} on:keydown={(e) => e.key === 'Escape' && closeAddRepoDialog()}>
    <div class="dialog" role="dialog" on:click|stopPropagation on:keydown|stopPropagation>
      <h4>Add GitHub Repository</h4>
      <input
        type="text"
        bind:value={newRepoURL}
        placeholder="https://github.com/user/repo"
        class="key-input"
        class:invalid={showNewRepoURLError}
        disabled={addRepoBusy}
      />
      {#if showNewRepoURLError}
        <p class="field-error">Enter a valid GitHub repository URL (https://github.com/owner/repo)</p>
      {/if}
      <div class="dialog-actions">
        <button type="button" class="btn-secondary" on:click={closeAddRepoDialog} disabled={addRepoBusy}>Cancel</button>
        <button type="button" class="btn-secondary" disabled={!newRepoURLValid || addRepoBusy} on:click={proceedAddRepo}>
          {addRepoBusy ? 'Adding…' : 'Continue'}
        </button>
      </div>
    </div>
  </div>
{/if}

<ConfirmDialog
  show={addRepoUntrustedConfirmOpen}
  title="Add Untrusted Repository"
  message="Plugins from untrusted GitHub repositories can execute code on your machine. The repository owner can publish updates at any time. Only proceed if you trust the author and accept these risks."
  critical={true}
  requireCheckbox={true}
  confirmDisabled={addRepoBusy}
  checkboxLabel="I understand the risks and trust this repository. I take full responsibility."
  confirmLabel={addRepoBusy ? 'Adding…' : 'Add Repository'}
  cancelLabel="Cancel"
  on:confirm={confirmAddRepo}
  on:cancel={cancelAddRepoUntrustedConfirm}
>
  <ul slot="body" class="repo-risk-list">
    <li>Malicious or compromised plugin binaries</li>
    <li>Access to connection secrets (if requested by a plugin)</li>
    <li>Arbitrary network connections</li>
    <li>Unsigned or unverified plugin code</li>
  </ul>
</ConfirmDialog>

{#if pluginDetailsOpen && selectedGitHubPlugin}
  {@const detailsRelease = getSelectedRelease(selectedGitHubPlugin)}
  <Modal
    show={pluginDetailsOpen}
    title={selectedGitHubPlugin.name}
    contentClass="plugin-details-modal"
    on:close={closePluginDetails}
  >
    <div class="plugin-details">
      <div class="plugin-details-layout">
        <aside class="plugin-details-meta">
          <div class="plugin-details-badges">
            {#if selectedGitHubPlugin.installed}
              <span class="version-badge">Installed {formatInstalledVersion(selectedGitHubPlugin.installedVersion)}</span>
            {:else}
              <span class="badge not-installed">Not installed</span>
              {#if selectedGitHubPlugin.latestRelease}
                <span class="latest-release">Latest: {selectedGitHubPlugin.latestRelease}</span>
              {/if}
            {/if}
            {#if detailsRelease?.prerelease}
              <span class="badge warn">Pre-release</span>
            {/if}
          </div>

          {#if selectedGitHubPlugin.description}
            <p class="plugin-details-description">{selectedGitHubPlugin.description}</p>
          {/if}

          <div class="meta-stack">
            {#if selectedGitHubPlugin.author}
              <div class="meta-item">
                <span class="meta-label">Author</span>
                <span class="meta-value">{selectedGitHubPlugin.author}</span>
              </div>
            {/if}
            {#if selectedGitHubPlugin.license}
              <div class="meta-item">
                <span class="meta-label">License</span>
                <span class="meta-value">{selectedGitHubPlugin.license}</span>
              </div>
            {/if}
            {#if detailsRelease?.publishedAt || selectedGitHubPlugin.publishedAt}
              <div class="meta-item">
                <span class="meta-label">Published</span>
                <span class="meta-value">{formatPublishedDate(detailsRelease?.publishedAt || selectedGitHubPlugin.publishedAt)}</span>
              </div>
            {/if}
            {#if selectedGitHubPlugin.minCoreVersion}
              <div class="meta-item">
                <span class="meta-label">Min core</span>
                <span class="meta-value">{selectedGitHubPlugin.minCoreVersion}</span>
              </div>
            {/if}
          </div>

          {#if detailsRelease?.platforms?.length || selectedGitHubPlugin.platforms?.length}
            <div class="platform-section">
              <span class="meta-label">Platforms</span>
              <div class="platform-chips">
                {#each (detailsRelease?.platforms ?? selectedGitHubPlugin.platforms) as platform (platform.os + platform.arch)}
                  <span class="platform-chip">{platform.os}/{platform.arch}</span>
                {/each}
              </div>
            </div>
          {/if}
        </aside>

        <section class="plugin-details-readme">
          <span class="meta-label">README</span>
          <div class="readme-scroll">
            <GitHubReadmePanel
              markdown={selectedGitHubPlugin.readme}
              repositoryUrl={selectedGitHubPlugin.repositoryUrl}
              ref={getSelectedReleaseTag(selectedGitHubPlugin)}
            />
          </div>
        </section>
      </div>

      <div class="plugin-details-footer">
        <button type="button" class="btn-secondary" on:click={closePluginDetails}>Close</button>
        {#if selectedDetailsRepoURL && !selectedGitHubPlugin.installed && (detailsRelease?.platformSupported ?? selectedGitHubPlugin.platformSupported)}
          <button
            type="button"
            class="btn-secondary"
            disabled={githubPreviewBusy && previewingPluginId === selectedGitHubPlugin.id}
            on:click={() => {
              const plugin = selectedGitHubPlugin;
              const repoURL = selectedDetailsRepoURL;
              if (!plugin || !repoURL) return;
              closePluginDetails();
              void showGitHubInstallConfirm(repoURL, plugin);
            }}
          >
            {githubPreviewBusy && previewingPluginId === selectedGitHubPlugin.id ? 'Loading…' : 'Install'}
          </button>
        {/if}
      </div>
    </div>
  </Modal>
{/if}

{#if githubInstallConfirmOpen && githubInstallPreview}
  <div class="dialog-overlay" role="presentation" on:click={closeGitHubInstallConfirm} on:keydown={(e) => e.key === 'Escape' && closeGitHubInstallConfirm()}>
    <div class="dialog" role="dialog" on:click|stopPropagation on:keydown|stopPropagation>
      <h4>Install {githubInstallPreview.name}</h4>
      <pre class="install-preview">{githubInstallPreviewMessage}</pre>
      {#if githubInstallPreview.compatible === false}
        <div class="warning-box">
          <strong>Incompatible with this version of xQuakShell</strong>
          <ul>
            {#each githubInstallPreview.compatibilityIssues ?? [] as issue}
              <li>{issue}</li>
            {/each}
          </ul>
        </div>
      {/if}
      {#if githubInstallPreview.warnings?.length}
        <div class="warning-box">
          <strong>Security Warning</strong>
          <ul>
            {#each githubInstallPreview.warnings as warning}
              <li>{warning}</li>
            {/each}
          </ul>
        </div>
      {/if}
      <label class="checkbox-row">
        <input type="checkbox" bind:checked={githubInstallTrustConfirmed} />
        I understand the risks and trust this plugin. I take full responsibility.
      </label>
      {#if githubInstallPreview.requiresSecretAccess}
        <label class="checkbox-row">
          <input type="checkbox" bind:checked={githubGrantSecretAccess} />
          Grant access to secrets
        </label>
      {/if}
      {#if githubInstallPreview.requiresAuthProviderAccess}
        <label class="checkbox-row">
          <input type="checkbox" bind:checked={githubGrantAuthProviderAccess} />
          I understand this plugin can participate in SSH authentication
        </label>
      {/if}
      {#if githubInstallPreview.requiresTunnelProviderAccess}
        <label class="checkbox-row">
          <input type="checkbox" bind:checked={githubGrantTunnelProviderAccess} />
          I understand this plugin can route dynamic port-forward connections
        </label>
      {/if}
      {#if githubInstallPreview.arbitraryNetworkWarning}
        <div class="warning-box">
          <strong>Network access warning</strong>
          <p>This plugin can open TCP connections to arbitrary hosts.</p>
        </div>
        <label class="checkbox-row">
          <input type="checkbox" bind:checked={githubGrantArbitraryNetwork} />
          I understand this plugin can connect to any host on the internet
        </label>
      {/if}
      {#if githubInstallPreview.multiSessionWarning}
        <label class="checkbox-row">
          <input type="checkbox" bind:checked={githubGrantMultiSession} />
          Allow multi-session access (if required)
        </label>
      {/if}
      {#if githubInstallPreview.execAccessWarning}
        <div class="warning-box">
          <strong>Command execution warning</strong>
          <p>This plugin can run commands on the hosts you connect to, using your authenticated session.</p>
        </div>
        <label class="checkbox-row">
          <input type="checkbox" bind:checked={githubGrantExecAccess} />
          I understand this plugin can run commands on my sessions
        </label>
      {/if}
      <div class="dialog-actions">
        <button type="button" class="btn-secondary" on:click={closeGitHubInstallConfirm}>Cancel</button>
        <button type="button" class="btn-secondary" disabled={githubInstallPreview.compatible === false || !githubInstallTrustConfirmed || githubInstallBusy || githubPreviewBusy || (githubInstallPreview.requiresSecretAccess && !githubGrantSecretAccess) || (githubInstallPreview.requiresAuthProviderAccess && !githubGrantAuthProviderAccess) || (githubInstallPreview.requiresTunnelProviderAccess && !githubGrantTunnelProviderAccess) || (githubInstallPreview.arbitraryNetworkWarning && !githubGrantArbitraryNetwork) || (githubInstallPreview.multiSessionWarning && !githubGrantMultiSession) || (githubInstallPreview.execAccessWarning && !githubGrantExecAccess)} on:click={confirmGitHubInstall}>
          {githubInstallBusy ? 'Installing…' : 'Install'}
        </button>
      </div>
    </div>
  </div>
{/if}

{#if uninstallConfirmOpen && pendingUninstallPlugin}
  <div class="dialog-overlay" role="presentation" on:click={closeUninstallConfirm} on:keydown={(e) => e.key === 'Escape' && closeUninstallConfirm()}>
    <div class="dialog" role="dialog" on:click|stopPropagation on:keydown|stopPropagation>
      <h4>Uninstall {pendingUninstallPlugin.name}</h4>
      <p class="plugin-desc">This will remove the plugin and stop it if running.</p>
      <label class="checkbox-row">
        <input type="checkbox" bind:checked={removePluginData} />
        Also remove plugin data and settings
      </label>
      <div class="dialog-actions">
        <button type="button" class="btn-secondary" on:click={closeUninstallConfirm}>Cancel</button>
        <button type="button" class="btn-danger" on:click={confirmUninstall}>Uninstall</button>
      </div>
    </div>
  </div>
{/if}



<style>

  .plugin-settings { display: flex; flex-direction: column; gap: 12px; }

  .section-header { display: flex; align-items: center; justify-content: space-between; gap: 12px; flex-wrap: wrap; }

  .install-actions { display: flex; gap: 8px; flex-wrap: wrap; }

  .section-desc, .muted { color: var(--text-muted, #888); font-size: 13px; margin: 0; }

  .trust-panel { border: 1px solid var(--border-color, #333); border-radius: 8px; padding: 12px; display: flex; flex-direction: column; gap: 8px; }

  .trust-panel h4 { margin: 0; font-size: 14px; }

  .checkbox-row { display: flex; align-items: center; gap: 8px; font-size: 13px; }

  .trusted-keys { display: flex; flex-direction: column; gap: 6px; }

  .trusted-keys label { font-size: 12px; color: var(--text-muted, #888); }

  .key-row { display: flex; gap: 8px; flex-wrap: wrap; }

  .key-input { flex: 1; min-width: 200px; padding: 6px 8px; border-radius: 6px; border: 1px solid var(--border-color, #333); background: transparent; color: inherit; }
  .key-input.invalid { border-color: var(--danger, #f44747); }
  .field-error { margin: 0; font-size: 12px; color: var(--danger, #f44747); }

  .key-list { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 4px; font-size: 12px; }

  .key-list li { display: flex; align-items: center; gap: 8px; }

  .link-btn { background: none; border: none; color: #ff6b6b; cursor: pointer; font-size: 12px; padding: 0; }

  .plugin-list { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 8px; }
  .plugin-actions { display: flex; flex-direction: column; align-items: flex-end; gap: 6px; }
  .enable-toggle { font-size: 11px; }

  .plugin-row {

    display: flex; align-items: flex-start; justify-content: space-between; gap: 12px;

    border: 1px solid var(--border-color, #333); border-radius: 8px; padding: 10px 12px;

  }

  .plugin-title { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }

  .version, .plugin-meta, .plugin-desc { font-size: 12px; color: var(--text-muted, #888); }

  .plugin-desc { margin-top: 4px; }

  .badge.ok { display: inline-flex; align-items: center; gap: 4px; color: #6ecf8e; font-size: 11px; }

  .warn-line { color: #e6b35a; display: flex; align-items: center; gap: 6px; font-size: 12px; margin-top: 6px; }

  .error-text { color: #ff6b6b; font-size: 13px; margin: 0; }

  .tab-row { display: flex; gap: 8px; margin-bottom: 4px; }

  .tab-btn { display: inline-flex; align-items: center; gap: 6px; padding: 6px 12px; border-radius: 6px; border: 1px solid var(--border-color, #333); background: transparent; color: inherit; cursor: pointer; font-size: 13px; }

  .tab-btn.active { background: var(--border-color, #333); }

  .repo-list { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 10px; }

  .repo-item { border: 1px solid var(--border-color, #333); border-radius: 8px; padding: 12px; display: flex; flex-direction: column; gap: 8px; }

  .repo-info { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }

  .repo-url { font-size: 12px; color: var(--text-muted, #888); }

  .badge.warn { color: #e6b35a; font-size: 11px; }
  .badge.not-installed { color: var(--danger, #f44747); font-size: 11px; font-weight: 600; }
  .latest-release { color: var(--text-muted, #888); font-size: 11px; }

  .plugins-list { display: flex; flex-direction: column; gap: 8px; margin-top: 8px; padding-top: 8px; border-top: 1px solid var(--border-color, #333); }

  .plugin-card { border: 1px solid var(--border-color, #333); border-radius: 6px; padding: 10px; display: flex; flex-direction: column; gap: 8px; }

  .version-select-row { display: flex; align-items: center; gap: 8px; font-size: 12px; }
  .version-select-row label { color: var(--text-muted, #888); min-width: 52px; }
  .version-select {
    flex: 1;
    min-width: 0;
    padding: 5px 8px;
    border-radius: 6px;
    border: 1px solid var(--border-color, #333);
    background: transparent;
    color: inherit;
    font-size: 12px;
  }

  .repo-actions, .plugin-actions-row { display: flex; gap: 8px; flex-wrap: wrap; }
  .plugin-actions-row { margin-top: 10px; padding-top: 10px; border-top: 1px solid var(--border-color, #333); }

  :global(.modal-content.plugin-details-modal) {
    width: min(1400px, 94vw);
    max-width: min(1400px, 94vw);
    min-width: min(1000px, 94vw);
    max-height: 90vh;
  }

  .plugin-details { display: flex; flex-direction: column; gap: 16px; flex: 1; min-height: 0; }
  .plugin-details-layout {
    display: grid;
    grid-template-columns: 280px minmax(0, 1fr);
    gap: 28px;
    align-items: stretch;
    flex: 1;
    min-height: 520px;
  }
  .plugin-details-meta {
    display: flex;
    flex-direction: column;
    gap: 14px;
    min-width: 0;
  }
  .plugin-details-readme {
    display: flex;
    flex-direction: column;
    gap: 8px;
    min-width: 0;
    min-height: 0;
  }
  .readme-scroll {
    flex: 1;
    min-height: 0;
    overflow: auto;
    padding: 14px 16px;
    border-radius: 6px;
    border: 1px solid var(--border-color, #333);
    background: var(--bg-tertiary, rgba(0, 0, 0, 0.2));
  }

  .plugin-details-badges { display: flex; align-items: center; gap: 8px; flex-wrap: wrap; }
  .version-badge {
    display: inline-flex;
    align-items: center;
    padding: 2px 8px;
    border-radius: 999px;
    border: 1px solid var(--border-color, #333);
    font-size: 12px;
    color: var(--text-primary);
    background: var(--bg-tertiary, rgba(255,255,255,0.04));
  }
  .plugin-details-description { margin: 0; font-size: 13px; line-height: 1.5; color: var(--text-primary); }
  .meta-stack { display: flex; flex-direction: column; gap: 12px; }
  .meta-item { display: flex; flex-direction: column; gap: 4px; }
  .meta-label { font-size: 11px; text-transform: uppercase; letter-spacing: 0.04em; color: var(--text-muted, #888); }
  .meta-value { font-size: 13px; color: var(--text-primary); }
  .platform-section { display: flex; flex-direction: column; gap: 8px; }
  .platform-chips { display: flex; flex-wrap: wrap; gap: 6px; }
  .platform-chip {
    display: inline-flex;
    align-items: center;
    padding: 3px 8px;
    border-radius: 999px;
    border: 1px solid var(--border-color, #333);
    font-size: 11px;
    color: var(--text-primary);
  }
  .plugin-details-footer { display: flex; justify-content: flex-end; gap: 8px; padding-top: 4px; border-top: 1px solid var(--border-color, #333); }

  @media (max-width: 960px) {
    :global(.modal-content.plugin-details-modal) {
      min-width: 0;
      width: 96vw;
      max-width: 96vw;
    }
  }

  @media (max-width: 760px) {
    .plugin-details-layout {
      grid-template-columns: 1fr;
      min-height: 0;
    }
    .plugin-details-readme {
      min-height: 280px;
    }
    .readme-scroll {
      min-height: 240px;
      max-height: 50vh;
    }
  }

  .btn-danger { border: 1px solid #ff6b6b; color: #ff6b6b; background: transparent; border-radius: 6px; padding: 6px 10px; cursor: pointer; font-size: 12px; }

  .dialog-overlay { position: fixed; inset: 0; background: rgba(0,0,0,0.6); display: flex; align-items: center; justify-content: center; z-index: 1000; }

  .dialog { background: var(--bg, #1e1e1e); border: 1px solid var(--border-color, #333); border-radius: 8px; padding: 16px; width: min(520px, 92vw); display: flex; flex-direction: column; gap: 10px; }

  .dialog-large { width: min(760px, 92vw); max-height: 80vh; overflow: auto; }

  .dialog-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 4px; }

  .warning-box { background: rgba(255, 107, 107, 0.1); border: 1px solid #ff6b6b; border-radius: 6px; padding: 10px; font-size: 13px; }

  .warning-box ul { margin: 6px 0 0; padding-left: 18px; }

  .readme { white-space: pre-wrap; font-size: 12px; max-height: 320px; overflow: auto; background: rgba(0,0,0,0.2); padding: 10px; border-radius: 6px; }

  .install-preview { white-space: pre-wrap; font-size: 12px; max-height: 240px; overflow: auto; background: rgba(0,0,0,0.2); padding: 10px; border-radius: 6px; margin: 0; }

  :global(.spinning) {
    animation: plugin-refresh-spin 0.8s linear infinite;
  }

  @keyframes plugin-refresh-spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }

</style>

