<script lang="ts">
  import SettingsSection from './SettingsSection.svelte';
  import { CONFLICT_ACTIONS } from '../transfer/conflictActions';
  import { t } from '../../i18n/messages';
  import type { SettingsSearchViewState } from '../settingsSearch';
  import type { SettingsDraft } from './settingsDraft';

  export let view: SettingsSearchViewState;
  // Owns externalEditorPath and the two default conflict actions.
  export let draft: SettingsDraft;
</script>

<SettingsSection tab="files" section="editor" {view}>
  <div class="section">
    <h4>{$t('settings.files.editor.title')}</h4>
    <p class="section-desc">{$t('settings.files.editor.desc')}</p>
    <label class="setting-row">
      <span>{$t('settings.files.editor.path')}</span>
      <input type="text" bind:value={draft.externalEditorPath} placeholder={$t('settings.files.editor.placeholder')} />
    </label>
  </div>
</SettingsSection>

<SettingsSection tab="files" section="conflicts" {view}>
  <div class="section">
    <h4>{$t('settings.files.conflicts.title')}</h4>
    <p class="section-desc">{$t('settings.files.conflicts.desc')}</p>
    <label class="setting-row">
      <span>{$t('settings.files.conflicts.upload')}</span>
      <select bind:value={draft.defaultUploadExistsAction}>
        <option value="ask">{$t('transfer.conflict.ask')}</option>
        {#each CONFLICT_ACTIONS as a}
          <option value={a.value}>{$t(a.labelKey)}</option>
        {/each}
      </select>
    </label>
    <label class="setting-row">
      <span>{$t('settings.files.conflicts.download')}</span>
      <select bind:value={draft.defaultDownloadExistsAction}>
        <option value="ask">{$t('transfer.conflict.ask')}</option>
        {#each CONFLICT_ACTIONS as a}
          <option value={a.value}>{$t(a.labelKey)}</option>
        {/each}
      </select>
    </label>
  </div>
</SettingsSection>
