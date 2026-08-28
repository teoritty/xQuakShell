<script lang="ts">
  import SettingsSection from './SettingsSection.svelte';
  import { findHotkeyConflict } from './hotkeyConflicts';
  import { parseHotkeyEvent } from '../../hotkeys/hotkeys';
  import { DEFAULT_LOCAL_TERMINAL_HOTKEY, DEFAULT_SESSION_HOTKEYS } from '../../api/settings';
  import { t } from '../../i18n/messages';
  import type { SettingsSearchViewState } from '../settingsSearch';
  import type { SettingsDraft } from './settingsDraft';

  export let view: SettingsSearchViewState;
  // Owns the four sessionHotkey* fields and the local terminal's. The conflict message is bound
  // out because the dialog refuses to save while one stands.
  export let draft: SettingsDraft;
  export let conflict: string;

  type HotkeyField = 'create' | 'next' | 'prev' | 'close' | 'localTerminal';

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
    if (field === 'localTerminal') draft.localTerminalHotkey = key;
    conflict = findHotkeyConflict(draft, $t);
  }

  function resetToDefault() {
    draft.sessionHotkeyCreate = DEFAULT_SESSION_HOTKEYS.create;
    draft.sessionHotkeyNext = DEFAULT_SESSION_HOTKEYS.next;
    draft.sessionHotkeyPrev = DEFAULT_SESSION_HOTKEYS.prev;
    draft.sessionHotkeyClose = DEFAULT_SESSION_HOTKEYS.close;
    draft.localTerminalHotkey = DEFAULT_LOCAL_TERMINAL_HOTKEY;
    conflict = '';
  }
</script>

<SettingsSection tab="hotkeys" section="session" {view}>
  <div class="section">
    <h4>{$t('settings.hotkeys.session.title')}</h4>
    <p class="section-desc">{$t('settings.hotkeys.session.desc')}</p>
    <label class="setting-row">
      <span>{$t('settings.hotkeys.field.create')}</span>
      <input class="hotkey-input" type="text" bind:value={draft.sessionHotkeyCreate} on:keydown={(e) => captureHotkey(e, 'create')} />
    </label>
    <label class="setting-row">
      <span>{$t('settings.hotkeys.field.next')}</span>
      <input class="hotkey-input" type="text" bind:value={draft.sessionHotkeyNext} on:keydown={(e) => captureHotkey(e, 'next')} />
    </label>
    <label class="setting-row">
      <span>{$t('settings.hotkeys.field.prev')}</span>
      <input class="hotkey-input" type="text" bind:value={draft.sessionHotkeyPrev} on:keydown={(e) => captureHotkey(e, 'prev')} />
    </label>
    <label class="setting-row">
      <span>{$t('settings.hotkeys.field.close')}</span>
      <input class="hotkey-input" type="text" bind:value={draft.sessionHotkeyClose} on:keydown={(e) => captureHotkey(e, 'close')} />
    </label>
    <label class="setting-row">
      <span>{$t('settings.hotkeys.field.newLocalTerminal')}</span>
      <input class="hotkey-input" type="text" bind:value={draft.localTerminalHotkey} on:keydown={(e) => captureHotkey(e, 'localTerminal')} />
    </label>
    {#if conflict}
      <div class="hotkey-conflict">{conflict}</div>
    {/if}
    <div class="hotkey-actions">
      <button class="secondary" type="button" on:click={resetToDefault}>{$t('settings.hotkeys.session.reset')}</button>
    </div>
  </div>
</SettingsSection>
