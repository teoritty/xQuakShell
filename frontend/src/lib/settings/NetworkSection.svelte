<script lang="ts">
  import SettingsSection from './SettingsSection.svelte';
  import { t } from '../../i18n/messages';
  import type { SettingsSearchViewState } from '../settingsSearch';
  import type { SettingsDraft } from './settingsDraft';

  export let view: SettingsSearchViewState;
  // Owns the ping fields and the three transfer fields.
  export let draft: SettingsDraft;
</script>

<SettingsSection tab="network" section="ping" {view}>
  <div class="section">
    <h4>{$t('settings.network.ping.title')}</h4>
    <p class="section-desc">{$t('settings.network.ping.desc')}</p>
    <label class="checkbox-row">
      <input type="checkbox" bind:checked={draft.pingEnabled} />
      {$t('settings.network.ping.enable')}
    </label>
    <label class="setting-row">
      <span>{$t('settings.network.ping.mode')}</span>
      <select bind:value={draft.pingMode} disabled={!draft.pingEnabled}>
        <option value="on_change">{$t('settings.network.ping.mode.onChange')}</option>
        <option value="interval">{$t('settings.network.ping.mode.interval')}</option>
      </select>
    </label>
    <label class="setting-row">
      <span>{$t('settings.network.ping.interval')}</span>
      <input
        type="number"
        bind:value={draft.pingIntervalSeconds}
        min="5"
        max="300"
        disabled={draft.pingMode !== 'interval' || !draft.pingEnabled}
      />
    </label>
    <label class="setting-row">
      <span>{$t('settings.network.ping.maxConcurrent')}</span>
      <input
        type="number"
        bind:value={draft.maxConcurrentPings}
        min="1"
        max="64"
        disabled={!draft.pingEnabled}
      />
    </label>
  </div>
</SettingsSection>

<SettingsSection tab="network" section="transfer" {view}>
  <div class="section">
    <h4>{$t('settings.network.transfer.title')}</h4>
    <label class="setting-row">
      <span>{$t('settings.network.transfer.speedLimit')}</span>
      <input type="number" bind:value={draft.transferSpeedLimitKbps} min="0" placeholder={$t('settings.network.transfer.speedLimit.placeholder')} />
    </label>
    <label class="setting-row">
      <span>{$t('settings.network.transfer.timeout')}</span>
      <input type="number" bind:value={draft.connectionTimeoutSeconds} min="5" max="300" />
    </label>
    <label class="setting-row">
      <span>{$t('settings.network.transfer.maxConcurrent')}</span>
      <input type="number" bind:value={draft.maxConcurrentTransfers} min="1" max="16" />
    </label>
  </div>
</SettingsSection>
