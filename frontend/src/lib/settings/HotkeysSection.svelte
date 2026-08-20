<script lang="ts">
  import SettingsSection from './SettingsSection.svelte';
  import { findHotkeyConflict } from './hotkeyConflicts';
  import { parseHotkeyEvent } from '../../hotkeys/hotkeys';
  import { DEFAULT_SESSION_HOTKEYS } from '../../api/settings';
  import type { SettingsSearchViewState } from '../settingsSearch';
  import type { SettingsDraft } from './settingsDraft';

  export let view: SettingsSearchViewState;
  // Owns the four sessionHotkey* fields. The conflict message is bound out because the dialog
  // refuses to save while one stands.
  export let draft: SettingsDraft;
  export let conflict: string;

  type HotkeyField = 'create' | 'next' | 'prev' | 'close';

  // The field records the combination rather than the characters it produces, so the keystroke must
  // not also reach the field as text or the hotkey editor would type into itself.
  function captureHotkey(e: KeyboardEvent, field: HotkeyField) {
    e.preventDefault();
    e.stopPropagation();
    const key = parseHotkeyEvent(e);
    if (!key) return;
    if (field === 'create') draft.sessionHotkeyCreate = key;
    if (field === 'next') draft.sessionHotkeyNext = key;
    if (field === 'prev') draft.sessionHotkeyPrev = key;
    if (field === 'close') draft.sessionHotkeyClose = key;
    conflict = findHotkeyConflict(draft);
  }

  function resetToDefault() {
    draft.sessionHotkeyCreate = DEFAULT_SESSION_HOTKEYS.create;
    draft.sessionHotkeyNext = DEFAULT_SESSION_HOTKEYS.next;
    draft.sessionHotkeyPrev = DEFAULT_SESSION_HOTKEYS.prev;
    draft.sessionHotkeyClose = DEFAULT_SESSION_HOTKEYS.close;
    conflict = '';
  }
</script>

<SettingsSection tab="hotkeys" section="session" {view}>
  <div class="section">
    <h4>Session hotkeys</h4>
    <p class="section-desc">Click a field and press a key combination.</p>
    <label class="setting-row">
      <span>Create session</span>
      <input class="hotkey-input" type="text" bind:value={draft.sessionHotkeyCreate} on:keydown={(e) => captureHotkey(e, 'create')} />
    </label>
    <label class="setting-row">
      <span>Next session tab</span>
      <input class="hotkey-input" type="text" bind:value={draft.sessionHotkeyNext} on:keydown={(e) => captureHotkey(e, 'next')} />
    </label>
    <label class="setting-row">
      <span>Previous session tab</span>
      <input class="hotkey-input" type="text" bind:value={draft.sessionHotkeyPrev} on:keydown={(e) => captureHotkey(e, 'prev')} />
    </label>
    <label class="setting-row">
      <span>Close active session</span>
      <input class="hotkey-input" type="text" bind:value={draft.sessionHotkeyClose} on:keydown={(e) => captureHotkey(e, 'close')} />
    </label>
    {#if conflict}
      <div class="hotkey-conflict">{conflict}</div>
    {/if}
    <div class="hotkey-actions">
      <button class="secondary" type="button" on:click={resetToDefault}>Reset to defaults</button>
    </div>
  </div>
</SettingsSection>
