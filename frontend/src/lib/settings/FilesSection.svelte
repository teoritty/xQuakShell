<script lang="ts">
  import SettingsSection from './SettingsSection.svelte';
  import { CONFLICT_ACTIONS } from '../transfer/conflictActions';
  import type { SettingsSearchViewState } from '../settingsSearch';
  import type { SettingsDraft } from './settingsDraft';

  export let view: SettingsSearchViewState;
  // Owns externalEditorPath and the two default conflict actions.
  export let draft: SettingsDraft;
</script>

<SettingsSection tab="files" section="editor" {view}>
  <div class="section">
    <h4>External editor</h4>
    <p class="section-desc">A remote file you edit is downloaded, opened in this editor, and re-uploaded when you save.</p>
    <label class="setting-row">
      <span>Editor path</span>
      <input type="text" bind:value={draft.externalEditorPath} placeholder="e.g. code, notepad.exe, C:\...\gvim.exe" />
    </label>
  </div>
</SettingsSection>

<SettingsSection tab="files" section="conflicts" {view}>
  <div class="section">
    <h4>When a file already exists</h4>
    <p class="section-desc">"Ask every time" shows the conflict dialog; any other choice applies silently. Picking an action in that dialog without "Apply to current queue only" also changes these.</p>
    <label class="setting-row">
      <span>Uploads and local copies</span>
      <select bind:value={draft.defaultUploadExistsAction}>
        <option value="ask">Ask every time</option>
        {#each CONFLICT_ACTIONS as a}
          <option value={a.value}>{a.label}</option>
        {/each}
      </select>
    </label>
    <label class="setting-row">
      <span>Downloads</span>
      <select bind:value={draft.defaultDownloadExistsAction}>
        <option value="ask">Ask every time</option>
        {#each CONFLICT_ACTIONS as a}
          <option value={a.value}>{a.label}</option>
        {/each}
      </select>
    </label>
  </div>
</SettingsSection>
