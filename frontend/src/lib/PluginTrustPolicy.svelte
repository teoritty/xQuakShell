<script lang="ts">
  // The plugin trust policy: every control here answers one question — how much is this
  // installation willing to trust a plugin before it runs. Nothing else in Settings reads any of
  // this state, and it saves on its own rather than through the dialog's Save button, which is why
  // it is a component and not inline markup.
  import { onMount } from 'svelte';
  import {
    getPluginSettings,
    savePluginSettings,
    generatePluginPublisherKeyPair,
    type PluginSettings,
  } from '../api/plugins';
  import { t } from '../i18n/messages';

  // onError hands failures up to the screen, which owns the one place errors are shown. A second
  // error line inside this box would compete with that one for the user's attention.
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
      onError(e instanceof Error ? e.message : $t('plugins.trust.loadFailed'));
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
      onError(e instanceof Error ? e.message : $t('plugins.trust.saveFailed'));
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
      onError(e instanceof Error ? e.message : $t('plugins.trust.keygenFailed'));
    }
  }
</script>

<!-- No heading and no box of its own: this is the body of a Settings section, which supplies both.
     A second border inside the section's own divider read as a card that had lost its list. -->
<div class="trust-panel">
  {#if reauthPrompt}
    <div class="reauth" role="group" aria-label={$t('security.plugin.trust.reauth.aria')}>
      <p class="reauth-text">{$t('security.plugin.trust.reauth.text')}</p>
      <div class="key-row">
        <!-- svelte-ignore a11y-autofocus -->
        <input
          type="password"
          class="key-input"
          autofocus
          bind:value={masterPassword}
          placeholder={$t('vault.field.master')}
          aria-label={$t('vault.field.master')}
          on:keydown={(e) => e.key === 'Enter' && confirmReauth()}
        />
        <button type="button" class="btn-secondary" disabled={busy || !masterPassword} on:click={confirmReauth}>
          {$t('common.confirm')}
        </button>
        <button type="button" class="btn-secondary" disabled={busy} on:click={cancelReauth}>{$t('common.cancel')}</button>
      </div>
      {#if reauthFailed}
        <p class="reauth-error">{$t('security.plugin.trust.reauth.failed')}</p>
      {/if}
    </div>
  {/if}

  <label class="checkbox-row">
    <input type="checkbox" bind:checked={settings.requireSignedPlugins} on:change={() => save()} />
    {$t('security.plugin.trust.requireSigned')}
  </label>

  <label class="checkbox-row">
    <input type="checkbox" bind:checked={settings.allowUnsandboxedFallback} on:change={() => save()} />
    {$t('security.plugin.trust.allowUnsandboxed')}
  </label>
  <p class="setting-hint">{$t('security.plugin.trust.allowUnsandboxed.hint')}</p>

  <div class="trusted-keys">
    <label for="trusted-key">{$t('plugins.trust.keysLabel')}</label>
    <div class="key-row">
      <input id="trusted-key" class="key-input" bind:value={newTrustedKey} placeholder={$t('plugins.trust.keyPlaceholder')} />
      <button type="button" class="btn-secondary" disabled={busy} on:click={addTrustedKey}>{$t('common.add')}</button>
      <button type="button" class="btn-secondary" disabled={busy} on:click={generatePublisherKeys}>{$t('plugins.trust.generatePair')}</button>
    </div>
    {#if settings.trustedPublisherKeys.length > 0}
      <ul class="key-list">
        {#each settings.trustedPublisherKeys as key (key)}
          <li>
            <code>{key.slice(0, 24)}…</code>
            <button type="button" class="link-btn" on:click={() => removeTrustedKey(key)}>{$t('common.remove')}</button>
          </li>
        {/each}
      </ul>
    {/if}
  </div>
</div>

<style>
  .trust-panel { display: flex; flex-direction: column; gap: 9px; }
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
