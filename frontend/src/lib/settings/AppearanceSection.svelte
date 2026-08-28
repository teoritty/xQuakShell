<script lang="ts">
  import { onMount } from 'svelte';
  import SettingsSection from './SettingsSection.svelte';
  import { TERMINAL_FONT_STACKS } from './terminalFonts';
  import { applyUiScalePercent, normalizeUiScalePercent, UI_SCALE_PRESETS } from '../uiScale';
  import { listLocales, type LocaleInfo } from '../../api/locale';
  import { listLocalShells, type ShellOption } from '../../api/localTerminal';
  import { applyLocale } from '../../i18n/apply';
  import { t } from '../../i18n/messages';
  import type { SettingsSearchViewState } from '../settingsSearch';
  import type { SettingsDraft } from './settingsDraft';

  export let view: SettingsSearchViewState;
  // Owns theme, language, uiScalePercent, the three terminal font fields and the local shell.
  export let draft: SettingsDraft;

  let locales: LocaleInfo[] = [];
  // Only the shells this machine actually has. The list is built on the Go side, and the value
  // stored is one of its ids - never a path the user could type, which is what keeps the setting
  // a choice rather than an instruction.
  let shells: ShellOption[] = [];

  onMount(async () => {
    locales = await listLocales();
    shells = await listLocalShells();
  });

  // Like the scale below, the language applies as soon as it is picked so the user can read the
  // dialog they are about to save from. The dialog restores what it opened with if they cancel.
  function handleLanguageChange() {
    void applyLocale(draft.language);
  }

  // The scale applies as soon as it is picked so the user can judge it, and the dialog restores the
  // value it opened with if they cancel. Saving is what makes it permanent.
  function handleUiScaleChange() {
    draft.uiScalePercent = normalizeUiScalePercent(Number(draft.uiScalePercent));
    applyUiScalePercent(draft.uiScalePercent);
  }
</script>

<SettingsSection tab="appearance" section="language" {view}>
  <div class="section">
    <h4>{$t('settings.appearance.language.title')}</h4>
    <p class="section-desc">{$t('settings.appearance.language.desc')}</p>
    <label class="setting-row">
      <span>{$t('settings.appearance.language.label')}</span>
      <select bind:value={draft.language} on:change={handleLanguageChange}>
        {#each locales as locale (locale.code)}
          <option value={locale.code}>{locale.name}</option>
        {/each}
      </select>
    </label>
  </div>
</SettingsSection>

<SettingsSection tab="appearance" section="theme" {view}>
  <div class="section">
    <h4>{$t('settings.appearance.theme.title')}</h4>
    <div class="theme-options">
      <label class="theme-option" class:selected={draft.theme === 'dark'}>
        <input type="radio" bind:group={draft.theme} value="dark" />
        <div class="theme-swatch dark-swatch"></div>
        <span>{$t('settings.appearance.theme.dark')}</span>
      </label>
    </div>
  </div>
</SettingsSection>

<SettingsSection tab="appearance" section="scale" {view}>
  <div class="section">
    <h4>{$t('settings.appearance.scale.title')}</h4>
    <p class="section-desc">{$t('settings.appearance.scale.desc')}</p>
    <label class="setting-row">
      <span>{$t('settings.appearance.scale.label')}</span>
      <select bind:value={draft.uiScalePercent} on:change={handleUiScaleChange}>
        {#each UI_SCALE_PRESETS as preset}
          <option value={preset}>{preset}%</option>
        {/each}
      </select>
    </label>
  </div>
</SettingsSection>

<SettingsSection tab="appearance" section="font" {view}>
  <div class="section">
    <h4>{$t('settings.appearance.font.title')}</h4>
    <label class="setting-row">
      <span>{$t('settings.appearance.font.family')}</span>
      <select bind:value={draft.terminalFontFamily}>
        {#each TERMINAL_FONT_STACKS as font}
          <option value={font} style="font-family: {font}">{font.split(',')[0].trim()}</option>
        {/each}
      </select>
    </label>
    <label class="setting-row">
      <span>{$t('settings.appearance.font.size')}</span>
      <input type="number" bind:value={draft.terminalFontSize} min="8" max="32" />
    </label>
    <label class="setting-row">
      <span>{$t('settings.appearance.font.color')}</span>
      <div class="color-picker-row">
        <input type="color" bind:value={draft.terminalFontColor} class="color-input" />
        <input type="text" bind:value={draft.terminalFontColor} class="color-hex" placeholder="#cccccc" />
      </div>
    </label>
    <div class="font-preview" style="font-family: {draft.terminalFontFamily}; font-size: calc({draft.terminalFontSize}px * var(--ui-scale)); color: {draft.terminalFontColor};">
      user@server:~$ ls -la
    </div>
  </div>
</SettingsSection>

<SettingsSection tab="appearance" section="localShell" {view}>
  <div class="section">
    <h4>{$t('settings.localShell.title')}</h4>
    <p class="section-desc">{$t('settings.localShell.desc')}</p>
    <label class="setting-row">
      <span>{$t('settings.localShell.shell')}</span>
      <select bind:value={draft.localTerminalShellId}>
        <option value="">{$t('settings.localShell.systemDefault')}</option>
        {#each shells as shell}
          <option value={shell.id}>{shell.name}</option>
        {/each}
      </select>
    </label>
  </div>
</SettingsSection>
