<script lang="ts">
  // The plugin trust policy, lifted out of PluginSettingsPanel.
  //
  // It moved because the panel is at its size budget and the sandbox toggle below had nowhere to
  // go, but it belongs here on its own merits: nothing else in the panel reads any of this state,
  // and every one of these controls answers the same question — how much is this installation
  // willing to trust a plugin before it runs.
  import { onMount } from 'svelte';
  import {
    getPluginSettings,
    savePluginSettings,
    generatePluginPublisherKeyPair,
    type PluginSettings,
  } from '../api/plugins';

  // onError hands failures back to the panel, which owns the one place errors are shown. A second
  // error line inside this box would be a worse UI than the one it replaced.
  export let onError: (message: string) => void = () => {};

  let settings: PluginSettings = { trustedPublisherKeys: [], requireSignedPlugins: false, allowUnsandboxedFallback: false };
  let newTrustedKey = '';
  let busy = false;

  // Set when the backend refused the last save for want of the master password. The backend
  // decides that, not this component: a copy of the rule here would drift from the Go one, and
  // the drifted copy is the one that lets a weakening change through.
  let reauthPrompt = false;
  let masterPassword = '';
  let reauthFailed = false;

  onMount(loadSettings);

  async function loadSettings() {
    try {
      settings = await getPluginSettings();
    } catch (e) {
      onError(e instanceof Error ? e.message : 'Failed to load plugin settings');
    }
  }

  async function save(password = '') {
    busy = true;
    try {
      const result = await savePluginSettings(settings, password);
      if (result.reauthRequired) {
        // A refusal after the user typed something means that something was wrong. The backend
        // will not say whether the password was wrong or the vault locked, on purpose.
        reauthFailed = password !== '';
        reauthPrompt = true;
        return;
      }
      reauthPrompt = false;
      reauthFailed = false;
      masterPassword = '';
    } catch (e) {
      onError(e instanceof Error ? e.message : 'Failed to save plugin settings');
    } finally {
      busy = false;
    }
  }

  async function confirmReauth() {
    if (!masterPassword) return;
    await save(masterPassword);
  }

  // Reverting the edit is the only honest way out: the controls are bound to `settings`, so
  // leaving them showing a change the vault never took would misreport the policy in force.
  async function cancelReauth() {
    reauthPrompt = false;
    reauthFailed = false;
    masterPassword = '';
    await loadSettings();
  }

  async function addTrustedKey() {
    const key = newTrustedKey.trim();
    if (!key) return;
    if (settings.trustedPublisherKeys.includes(key)) {
      newTrustedKey = '';
      return;
    }
    settings = { ...settings, trustedPublisherKeys: [...settings.trustedPublisherKeys, key] };
    newTrustedKey = '';
    await save();
  }

  async function removeTrustedKey(key: string) {
    settings = {
      ...settings,
      trustedPublisherKeys: settings.trustedPublisherKeys.filter((k) => k !== key),
    };
    await save();
  }

  async function generatePublisherKeys() {
    try {
      const pair = await generatePluginPublisherKeyPair();
      if (!pair.publicKey) return;
      newTrustedKey = pair.publicKey;
    } catch (e) {
      onError(e instanceof Error ? e.message : 'Key generation failed');
    }
  }
</script>

<div class="trust-panel">
  <h4>Trust policy</h4>

  {#if reauthPrompt}
    <div class="reauth" role="group" aria-label="Confirm trust change">
      <p class="reauth-text">
        This change lowers what a plugin has to prove before it runs. Enter your master password to
        confirm it.
      </p>
      <div class="key-row">
        <!-- svelte-ignore a11y-autofocus -->
        <input
          type="password"
          class="key-input"
          autofocus
          bind:value={masterPassword}
          placeholder="Master password"
          aria-label="Master password"
          on:keydown={(e) => e.key === 'Enter' && confirmReauth()}
        />
        <button type="button" class="btn-secondary" disabled={busy || !masterPassword} on:click={confirmReauth}>
          Confirm
        </button>
        <button type="button" class="btn-secondary" disabled={busy} on:click={cancelReauth}>Cancel</button>
      </div>
      {#if reauthFailed}
        <p class="reauth-error">Could not confirm. Check the password and that the vault is unlocked.</p>
      {/if}
    </div>
  {/if}

  <label class="checkbox-row">
    <input type="checkbox" bind:checked={settings.requireSignedPlugins} on:change={() => save()} />
    Require signed plugins from trusted publishers
  </label>

  <label class="checkbox-row">
    <input type="checkbox" bind:checked={settings.allowUnsandboxedFallback} on:change={() => save()} />
    Start a plugin unconfined if its sandbox cannot be applied
  </label>
  <p class="setting-hint">
    Off by default. When your system can isolate a plugin and the attempt fails, the plugin does not
    start — turning this on lets it run with your full access instead, and its row will say
    <strong>not sandboxed</strong>. It has no effect where the system cannot isolate plugins at all.
  </p>

  <div class="trusted-keys">
    <label for="trusted-key">Trusted publisher keys (base64 Ed25519 public keys)</label>
    <div class="key-row">
      <input id="trusted-key" class="key-input" bind:value={newTrustedKey} placeholder="Paste public key…" />
      <button type="button" class="btn-secondary" disabled={busy} on:click={addTrustedKey}>Add</button>
      <button type="button" class="btn-secondary" disabled={busy} on:click={generatePublisherKeys}>Generate pair</button>
    </div>
    {#if settings.trustedPublisherKeys.length > 0}
      <ul class="key-list">
        {#each settings.trustedPublisherKeys as key (key)}
          <li>
            <code>{key.slice(0, 24)}…</code>
            <button type="button" class="link-btn" on:click={() => removeTrustedKey(key)}>Remove</button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>

<style>
  .trust-panel { border: 1px solid var(--border-color); border-radius: 8px; padding: 12px; display: flex; flex-direction: column; gap: 8px; }
  .trust-panel h4 { margin: 0; font-size: 12px; }
  .reauth { border: 1px solid var(--accent); border-radius: 6px; padding: 10px; display: flex; flex-direction: column; gap: 8px; }
  .reauth-text { margin: 0; font-size: 12px; line-height: 1.4; }
  .reauth-error { margin: 0; font-size: 11px; color: var(--error-color, #e06c75); }
  .checkbox-row { display: flex; align-items: center; gap: 8px; font-size: 12px; }
  .setting-hint { margin: -4px 0 0 24px; font-size: 11px; color: var(--text-secondary); line-height: 1.4; }
  .trusted-keys { display: flex; flex-direction: column; gap: 6px; }
  .trusted-keys label { font-size: 12px; color: var(--text-secondary); }
  .key-row { display: flex; gap: 8px; flex-wrap: wrap; }
  .key-input { flex: 1; min-width: 200px; padding: 6px 8px; border-radius: 6px; border: 1px solid var(--border-color); background: transparent; color: inherit; }
  .key-list { list-style: none; padding: 0; margin: 0; display: flex; flex-direction: column; gap: 4px; font-size: 12px; }
  .key-list li { display: flex; align-items: center; gap: 8px; }
  .btn-secondary { padding: 6px 10px; border-radius: 6px; border: 1px solid var(--border-color); background: transparent; color: inherit; cursor: pointer; font-size: 12px; }
  .btn-secondary:disabled { opacity: 0.5; cursor: default; }
  .link-btn { background: none; border: none; padding: 0; color: var(--accent); cursor: pointer; font-size: 12px; }
</style>
