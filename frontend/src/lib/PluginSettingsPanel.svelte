<script lang="ts">

  import { onMount } from 'svelte';

  import ConfirmDialog from './ConfirmDialog.svelte';

  import {

    listPlugins,

    selectPluginSourceDir,

    selectPluginBundleFile,

    previewPluginInstall,

    installPlugin,

    pingPlugin,
    setPluginEnabled,

    getPluginSettings,

    savePluginSettings,

    generatePluginPublisherKeyPair,

    listGitHubRepositories,

    addGitHubRepository,

    removeGitHubRepository,

    setGitHubRepositoryTrust,

    fetchGitHubPlugins,

    previewGitHubPluginInstall,

    installGitHubPlugin,

    uninstallGitHubPlugin,

    type PluginInfo,

    type PluginInstallPreview,

    type PluginSettings,

    type GitHubRepository,

    type GitHubPluginMetadata,

    type GitHubPluginPreview,

  } from '../stores/api';

  import { Puzzle, ShieldAlert, BadgeCheck, FileArchive, FolderOpen, Github, RefreshCw } from 'lucide-svelte';



  let plugins: PluginInfo[] = [];

  let loading = true;

  let installPreview: PluginInstallPreview | null = null;

  let pendingSourcePath = '';

  let installConfirmOpen = false;

  let installBusy = false;

  let errorMessage = '';

  let pluginSettings: PluginSettings = { trustedPublisherKeys: [], requireSignedPlugins: false };

  let newTrustedKey = '';

  let settingsBusy = false;

  let activeTab: 'installed' | 'github' = 'installed';

  let repositories: GitHubRepository[] = [];

  let repoPlugins: Record<string, GitHubPluginMetadata[]> = {};

  let reposLoading = false;

  let addRepoDialogOpen = false;

  let newRepoURL = '';

  let newRepoTrusted = false;

  let githubInstallPreview: GitHubPluginPreview | null = null;

  let githubInstallConfirmOpen = false;

  let githubInstallTrustConfirmed = false;

  let githubGrantSecretAccess = false;

  let githubGrantMultiSession = false;

  let pendingGitHubRepoURL = '';

  let githubInstallBusy = false;

  let pluginDetailsOpen = false;

  let selectedGitHubPlugin: GitHubPluginMetadata | null = null;

  let uninstallConfirmOpen = false;

  let removePluginData = false;

  let pendingUninstallPlugin: GitHubPluginMetadata | null = null;



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

      await installPlugin(pendingSourcePath, installPreview?.requiresSecretAccess ?? false);

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

  async function loadGitHubRepositories() {
    reposLoading = true;
    try {
      repositories = await listGitHubRepositories();
      for (const repo of repositories) {
        await refreshRepoPlugins(repo.url);
      }
    } catch (e) {
      errorMessage = e instanceof Error ? e.message : 'Failed to load GitHub repositories';
    } finally {
      reposLoading = false;
    }
  }

  async function refreshRepoPlugins(repoURL: string) {
    try {
      const result = await fetchGitHubPlugins(repoURL);
      if (result?.plugins) {
        repoPlugins = { ...repoPlugins, [repoURL]: result.plugins };
      }
    } catch (e) {
      errorMessage = e instanceof Error ? e.message : 'Failed to fetch plugins';
    }
  }

  function showAddRepoDialog() {
    newRepoURL = '';
    newRepoTrusted = false;
    addRepoDialogOpen = true;
  }

  function closeAddRepoDialog() {
    addRepoDialogOpen = false;
  }

  async function confirmAddRepo() {
    if (!newRepoURL.trim()) return;
    errorMessage = '';
    try {
      await addGitHubRepository(newRepoURL.trim(), newRepoTrusted);
      addRepoDialogOpen = false;
      await loadGitHubRepositories();
    } catch (e) {
      errorMessage = e instanceof Error ? e.message : 'Failed to add repository';
    }
  }

  async function toggleRepoTrust(repo: GitHubRepository) {
    errorMessage = '';
    try {
      await setGitHubRepositoryTrust(repo.url, !repo.trusted);
      await loadGitHubRepositories();
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
      await loadGitHubRepositories();
    } catch (e) {
      errorMessage = e instanceof Error ? e.message : 'Failed to remove repository';
    }
  }

  function showPluginDetails(plugin: GitHubPluginMetadata) {
    selectedGitHubPlugin = plugin;
    pluginDetailsOpen = true;
  }

  function closePluginDetails() {
    pluginDetailsOpen = false;
    selectedGitHubPlugin = null;
  }

  async function showGitHubInstallConfirm(repoURL: string, plugin: GitHubPluginMetadata) {
    errorMessage = '';
    pendingGitHubRepoURL = repoURL;
    try {
      githubInstallPreview = await previewGitHubPluginInstall(repoURL);
      githubInstallTrustConfirmed = false;
      githubGrantSecretAccess = false;
      githubGrantMultiSession = false;
      githubInstallConfirmOpen = true;
      selectedGitHubPlugin = plugin;
    } catch (e) {
      errorMessage = e instanceof Error ? e.message : 'Failed to preview plugin';
    }
  }

  function closeGitHubInstallConfirm() {
    githubInstallConfirmOpen = false;
    githubInstallPreview = null;
    pendingGitHubRepoURL = '';
  }

  async function confirmGitHubInstall() {
    if (!pendingGitHubRepoURL || !githubInstallTrustConfirmed) return;
    githubInstallBusy = true;
    errorMessage = '';
    try {
      await installGitHubPlugin(
        pendingGitHubRepoURL,
        githubGrantSecretAccess,
        githubGrantMultiSession,
      );
      closeGitHubInstallConfirm();
      await refreshPlugins();
      await loadGitHubRepositories();
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
      await loadGitHubRepositories();
    } catch (e) {
      errorMessage = e instanceof Error ? e.message : 'Uninstall failed';
    }
  }

  function isPluginInstalled(pluginId: string): boolean {
    return plugins.some((p) => p.id === pluginId);
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

  </div>



  <p class="section-desc">

    Manage out-of-process plugins. Install from a folder or a signed `.xqs-plugin` bundle.

  </p>



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
            <button type="button" class="btn-secondary" on:click={() => refreshRepoPlugins(repo.url)}>
              <RefreshCw size={12} /> Refresh
            </button>
            <button type="button" class="btn-secondary" on:click={() => toggleRepoTrust(repo)}>
              {repo.trusted ? 'Untrust' : 'Trust'}
            </button>
            <button type="button" class="btn-danger" on:click={() => removeRepo(repo.url)}>Remove</button>
          </div>
          {#if repoPlugins[repo.url]?.length}
            <div class="plugins-list">
              {#each repoPlugins[repo.url] as plugin (plugin.id)}
                <div class="plugin-card">
                  <div class="plugin-title">
                    <strong>{plugin.name}</strong>
                    <span class="version">v{plugin.version}</span>
                  </div>
                  {#if plugin.description}
                    <p class="plugin-desc">{plugin.description}</p>
                  {/if}
                  {#if !plugin.platformSupported}
                    <p class="warn-line">Not compatible with your platform</p>
                  {/if}
                  <div class="plugin-actions-row">
                    <button type="button" class="btn-secondary" on:click={() => showPluginDetails(plugin)}>Details</button>
                    {#if plugin.installed || isPluginInstalled(plugin.id)}
                      <button type="button" class="btn-danger" on:click={() => showUninstallConfirm(plugin)}>Uninstall</button>
                    {:else if plugin.platformSupported}
                      <button type="button" class="btn-secondary" on:click={() => showGitHubInstallConfirm(repo.url, plugin)}>Install</button>
                    {/if}
                  </div>
                </div>
              {/each}
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}

  {/if}

</div>



<ConfirmDialog

  show={installConfirmOpen}

  title="Install plugin?"

  message={installMessage}

  critical={installPreview?.requiresSecretAccess ?? false}
  requireCheckbox={installPreview?.requiresSecretAccess ?? false}
  checkboxLabel="I understand this plugin will have access to connection secrets"

  confirmLabel={installBusy ? 'Installing…' : 'Install'}

  on:confirm={confirmInstall}

  on:cancel={cancelInstall}

/>

{#if addRepoDialogOpen}
  <div class="dialog-overlay" role="presentation" on:click={closeAddRepoDialog} on:keydown={(e) => e.key === 'Escape' && closeAddRepoDialog()}>
    <div class="dialog" role="dialog" on:click|stopPropagation on:keydown|stopPropagation>
      <h4>Add GitHub Repository</h4>
      <input type="text" bind:value={newRepoURL} placeholder="https://github.com/user/repo" class="key-input" />
      <label class="checkbox-row">
        <input type="checkbox" bind:checked={newRepoTrusted} />
        I trust this repository (skip some security warnings)
      </label>
      <div class="dialog-actions">
        <button type="button" class="btn-secondary" on:click={closeAddRepoDialog}>Cancel</button>
        <button type="button" class="btn-secondary" on:click={confirmAddRepo}>Add Repository</button>
      </div>
    </div>
  </div>
{/if}

{#if pluginDetailsOpen && selectedGitHubPlugin}
  <div class="dialog-overlay" role="presentation" on:click={closePluginDetails} on:keydown={(e) => e.key === 'Escape' && closePluginDetails()}>
    <div class="dialog dialog-large" role="dialog" on:click|stopPropagation on:keydown|stopPropagation>
      <h4>{selectedGitHubPlugin.name}</h4>
      <p class="plugin-meta">v{selectedGitHubPlugin.version} · {selectedGitHubPlugin.latestRelease}</p>
      {#if selectedGitHubPlugin.author}<p class="plugin-desc">Author: {selectedGitHubPlugin.author}</p>{/if}
      {#if selectedGitHubPlugin.license}<p class="plugin-desc">License: {selectedGitHubPlugin.license}</p>{/if}
      <p class="plugin-desc">Platforms: {selectedGitHubPlugin.platforms.map((p) => `${p.os}/${p.arch}`).join(', ')}</p>
      {#if selectedGitHubPlugin.readme}
        <pre class="readme">{selectedGitHubPlugin.readme}</pre>
      {/if}
      <div class="dialog-actions">
        <button type="button" class="btn-secondary" on:click={closePluginDetails}>Close</button>
      </div>
    </div>
  </div>
{/if}

{#if githubInstallConfirmOpen && githubInstallPreview}
  <div class="dialog-overlay" role="presentation" on:click={closeGitHubInstallConfirm} on:keydown={(e) => e.key === 'Escape' && closeGitHubInstallConfirm()}>
    <div class="dialog" role="dialog" on:click|stopPropagation on:keydown|stopPropagation>
      <h4>Install {githubInstallPreview.name}</h4>
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
      <label class="checkbox-row">
        <input type="checkbox" bind:checked={githubGrantMultiSession} />
        Allow multi-session access (if required)
      </label>
      <div class="dialog-actions">
        <button type="button" class="btn-secondary" on:click={closeGitHubInstallConfirm}>Cancel</button>
        <button type="button" class="btn-secondary" disabled={!githubInstallTrustConfirmed || githubInstallBusy || (githubInstallPreview.requiresSecretAccess && !githubGrantSecretAccess)} on:click={confirmGitHubInstall}>
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

  .repo-actions, .plugin-actions-row { display: flex; gap: 8px; flex-wrap: wrap; }

  .plugins-list { display: flex; flex-direction: column; gap: 8px; margin-top: 8px; padding-top: 8px; border-top: 1px solid var(--border-color, #333); }

  .plugin-card { border: 1px solid var(--border-color, #333); border-radius: 6px; padding: 10px; }

  .btn-danger { border: 1px solid #ff6b6b; color: #ff6b6b; background: transparent; border-radius: 6px; padding: 6px 10px; cursor: pointer; font-size: 12px; }

  .dialog-overlay { position: fixed; inset: 0; background: rgba(0,0,0,0.6); display: flex; align-items: center; justify-content: center; z-index: 1000; }

  .dialog { background: var(--bg, #1e1e1e); border: 1px solid var(--border-color, #333); border-radius: 8px; padding: 16px; width: min(520px, 92vw); display: flex; flex-direction: column; gap: 10px; }

  .dialog-large { width: min(760px, 92vw); max-height: 80vh; overflow: auto; }

  .dialog-actions { display: flex; justify-content: flex-end; gap: 8px; margin-top: 4px; }

  .warning-box { background: rgba(255, 107, 107, 0.1); border: 1px solid #ff6b6b; border-radius: 6px; padding: 10px; font-size: 13px; }

  .warning-box ul { margin: 6px 0 0; padding-left: 18px; }

  .readme { white-space: pre-wrap; font-size: 12px; max-height: 320px; overflow: auto; background: rgba(0,0,0,0.2); padding: 10px; border-radius: 6px; }

</style>

