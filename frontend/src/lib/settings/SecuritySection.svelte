<script lang="ts">
  import SettingsSection from './SettingsSection.svelte';
  import PluginTrustSection from './PluginTrustSection.svelte';
  import MasterPasswordSection from './MasterPasswordSection.svelte';
  import { t } from '../../i18n/messages';
  import type { SettingsSearchViewState } from '../settingsSearch';
  import type { SettingsDraft } from './settingsDraft';

  export let view: SettingsSearchViewState;
  // Owns the three lockout fields. Plugin trust is edited by PluginTrustSection, which writes
  // through its own reauthenticated path rather than through this dialog's save.
  export let draft: SettingsDraft;
</script>

<SettingsSection tab="security" section="masterPassword" {view}>
  <MasterPasswordSection />
</SettingsSection>

<SettingsSection tab="security" section="lockout" {view}>
  <div class="section">
    <h4>{$t('settings.security.lockout.title')}</h4>
    <label class="checkbox-row">
      <input type="checkbox" bind:checked={draft.lockoutEnabled} />
      {$t('settings.security.lockout.enable')}
    </label>
    <label class="setting-row">
      <span>{$t('settings.security.lockout.timeout')}</span>
      <input
        type="number"
        bind:value={draft.lockoutIdleMinutes}
        min="1"
        max="120"
        disabled={!draft.lockoutEnabled}
      />
    </label>
    <label class="checkbox-row">
      <input type="checkbox" bind:checked={draft.lockOnMinimize} />
      {$t('settings.security.lockout.onMinimize')}
    </label>
  </div>
</SettingsSection>

<SettingsSection tab="security" section="plugins" {view}>
  <PluginTrustSection />
</SettingsSection>
