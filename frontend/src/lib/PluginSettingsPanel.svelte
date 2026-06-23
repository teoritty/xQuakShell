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

    type PluginInfo,

    type PluginInstallPreview,

    type PluginSettings,

  } from '../stores/api';

  import { Puzzle, ShieldAlert, BadgeCheck, FileArchive, FolderOpen } from 'lucide-svelte';



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



  onMount(() => {

    refreshPlugins();

    loadPluginSettings();

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

</style>

