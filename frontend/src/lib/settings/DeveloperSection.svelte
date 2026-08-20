<script lang="ts">
  import SettingsSection from './SettingsSection.svelte';
  import type { SettingsSearchViewState } from '../settingsSearch';
  import type { SettingsDraft } from './settingsDraft';

  export let view: SettingsSearchViewState;
  // Owns debugLogWindowEnabled and debugLogLevel.
  export let draft: SettingsDraft;
</script>

<SettingsSection tab="about" section="developer" {view}>
  <div class="section">
    <h4>Developer</h4>
    <label class="checkbox-row">
      <input type="checkbox" bind:checked={draft.debugLogWindowEnabled} />
      Open debug log window
    </label>
    <label class="setting-row">
      <span>Minimum log level</span>
      <select bind:value={draft.debugLogLevel}>
        <option value="debug">Debug (most verbose)</option>
        <option value="info">Info</option>
        <option value="warn">Warning</option>
        <option value="error">Error</option>
      </select>
    </label>
  </div>
</SettingsSection>
