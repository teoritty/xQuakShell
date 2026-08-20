<script lang="ts">
  import SettingsSection from './SettingsSection.svelte';
  import { t } from '../../i18n/messages';
  import type { SettingsSearchViewState } from '../settingsSearch';
  import type { SettingsDraft } from './settingsDraft';

  export let view: SettingsSearchViewState;
  // Owns debugLogWindowEnabled and debugLogLevel.
  export let draft: SettingsDraft;
</script>

<SettingsSection tab="about" section="developer" {view}>
  <div class="section">
    <h4>{$t('settings.about.developer.title')}</h4>
    <label class="checkbox-row">
      <input type="checkbox" bind:checked={draft.debugLogWindowEnabled} />
      {$t('settings.about.developer.window')}
    </label>
    <label class="setting-row">
      <span>{$t('settings.about.developer.level')}</span>
      <select bind:value={draft.debugLogLevel}>
        <option value="debug">{$t('settings.about.developer.level.debug')}</option>
        <option value="info">{$t('settings.about.developer.level.info')}</option>
        <option value="warn">{$t('settings.about.developer.level.warn')}</option>
        <option value="error">{$t('settings.about.developer.level.error')}</option>
      </select>
    </label>
  </div>
</SettingsSection>
