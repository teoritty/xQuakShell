<script lang="ts">
  // The settings dialog owns the shell: the tab strip, the search box, and the fetch-edit-write
  // cycle around one draft record. Each tab's fields live in its own component under settings/,
  // and the defaults and mapping live in settings/settingsDraft.ts, which is what keeps this file
  // about the form rather than about every setting the application has.
  import { onDestroy, onMount } from 'svelte';
  import Modal from './Modal.svelte';
  import { getSettings, saveSettings } from '../actions/settingsActions';
  import AboutSection from './settings/AboutSection.svelte';
  import AppearanceSection from './settings/AppearanceSection.svelte';
  import AuditSection from './settings/AuditSection.svelte';
  import DeveloperSection from './settings/DeveloperSection.svelte';
  import FilesSection from './settings/FilesSection.svelte';
  import HotkeysSection from './settings/HotkeysSection.svelte';
  import NetworkSection from './settings/NetworkSection.svelte';
  import SecuritySection from './settings/SecuritySection.svelte';
  import SettingsSection from './settings/SettingsSection.svelte';
  import { findHotkeyConflict } from './settings/hotkeyConflicts';
  import {
    defaultSettingsDraft,
    draftFromSettings,
    draftToSettings,
  } from './settings/settingsDraft';
  import { getAuditSessionState } from '../api/audit';
  import { tabHasSearchMatches, type SettingsTabId } from './settingsSearch';
  import { applyUiScalePercent } from './uiScale';
  import { applyLocale } from '../i18n/apply';
  import {
    Shield,
    Palette,
    Info,
    Save,
    Wifi,
    FileEdit,
    Keyboard,
    FileText,
    Search,
  } from 'lucide-svelte';

  export let show = false;
  export let initialTab: SettingsTabId = 'about';

  let activeTab: SettingsTabId = 'about';
  let loading = true;
  let saving = false;
  let searchQuery = '';
  let searchPinnedTab: SettingsTabId | null = null;

  let draft = defaultSettingsDraft();
  let hotkeyConflict = '';
  // Not part of the draft: secret logging is session state the backend owns, while the scale and
  // language are remembered only so cancelling can undo their live previews.
  let auditLogSecrets = false;
  let uiScaleAtOpen = draft.uiScalePercent;
  let languageAtOpen = draft.language;

  onMount(() => {
    const rt = (window as any).runtime;
    if (!rt?.EventsOn) return;
    return rt.EventsOn('DebugLogWindowChanged', (data: { enabled?: boolean }) => {
      draft.debugLogWindowEnabled = data?.enabled ?? false;
    });
  });

  onDestroy(() => {
    const rt = (window as any).runtime;
    if (rt?.EventsOff) rt.EventsOff('DebugLogWindowChanged');
  });

  const tabs: { id: SettingsTabId; label: string; icon: typeof Shield }[] = [
    { id: 'about', label: 'About', icon: Info },
    { id: 'appearance', label: 'Appearance', icon: Palette },
    { id: 'audit', label: 'Audit Log', icon: FileText },
    { id: 'files', label: 'Files', icon: FileEdit },
    { id: 'hotkeys', label: 'Hotkeys', icon: Keyboard },
    { id: 'network', label: 'Network', icon: Wifi },
    { id: 'security', label: 'Security', icon: Shield },
  ];

  $: isSearching = searchQuery.trim().length > 0;
  $: view = { isSearching, activeTab, searchQuery, searchPinnedTab };
  $: visibleTabs = isSearching
    ? tabs.filter((tab) => tabHasSearchMatches(tab.id, searchQuery))
    : tabs;

  let settingsWasOpen = false;
  $: if (show && !settingsWasOpen) {
    settingsWasOpen = true;
    activeTab = initialTab;
    searchQuery = '';
    searchPinnedTab = null;
    void loadSettings();
  }
  $: if (!show) {
    settingsWasOpen = false;
  }

  function handleTabClick(tabId: SettingsTabId) {
    if (isSearching) {
      searchPinnedTab = searchPinnedTab === tabId ? null : tabId;
      activeTab = tabId;
      return;
    }
    activeTab = tabId;
    searchPinnedTab = null;
  }

  function handleSearchInput() {
    if (!isSearching) {
      searchPinnedTab = null;
    }
  }

  async function loadSettings() {
    loading = true;
    draft = draftFromSettings(await getSettings());
    uiScaleAtOpen = draft.uiScalePercent;
    languageAtOpen = draft.language;
    const sessionState = await getAuditSessionState();
    auditLogSecrets = sessionState?.logSecretsEnabled ?? false;
    hotkeyConflict = '';
    loading = false;
  }

  // The scale and the language both preview live while the dialog is open, so cancelling has to
  // put back what it opened with rather than leave a preview standing as if it had been saved.
  function closeSettings() {
    applyUiScalePercent(uiScaleAtOpen);
    if (draft.language !== languageAtOpen) {
      void applyLocale(languageAtOpen);
    }
    show = false;
  }

  async function handleSave() {
    hotkeyConflict = findHotkeyConflict(draft);
    if (hotkeyConflict) return;
    saving = true;
    await saveSettings(draftToSettings(draft));
    window.dispatchEvent(new CustomEvent('app-settings-updated'));
    uiScaleAtOpen = draft.uiScalePercent;
    languageAtOpen = draft.language;
    saving = false;
    show = false;
  }
</script>

{#if show}
  <Modal title="Settings" show={true} contentClass="settings-modal" on:close={closeSettings}>
    <svelte:fragment slot="header-center">
      <div class="settings-search-wrap">
        <Search size={13} />
        <input
          type="text"
          class="settings-search-input"
          placeholder="Search settings..."
          bind:value={searchQuery}
          on:input={handleSearchInput}
        />
      </div>
    </svelte:fragment>

    <div class="settings-layout">
      <div class="settings-tabs">
        {#each visibleTabs as tab}
          <button
            class="tab-item"
            class:active={isSearching ? searchPinnedTab === tab.id : activeTab === tab.id}
            on:click={() => handleTabClick(tab.id)}
          >
            <svelte:component this={tab.icon} size={14} />
            {tab.label}
          </button>
        {/each}
      </div>

      <div class="settings-content">
        {#if loading}
          <div class="settings-loading">Loading...</div>
        {:else if isSearching && visibleTabs.length === 0}
          <div class="settings-loading">No matching settings</div>
        {:else}
          <SettingsSection tab="about" section="info" {view}>
            <AboutSection bind:updateCheckOnStartup={draft.updateCheckOnStartup} />
          </SettingsSection>
          <DeveloperSection {view} bind:draft />
          <AppearanceSection {view} bind:draft />
          <AuditSection {view} bind:draft bind:auditLogSecrets />
          <FilesSection {view} bind:draft />
          <HotkeysSection {view} bind:draft bind:conflict={hotkeyConflict} />
          <NetworkSection {view} bind:draft />
          <SecuritySection {view} bind:draft />
        {/if}
      </div>
    </div>

    <div class="settings-footer">
      <div class="settings-footer-actions">
        <button class="secondary" on:click={closeSettings}>Cancel</button>
        <button class="primary" on:click={handleSave} disabled={saving}>
          <Save size={13} />
          {saving ? 'Saving...' : 'Save'}
        </button>
      </div>
    </div>
  </Modal>
{/if}

<style>
  .settings-search-wrap {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    padding: 0 8px;
    background: var(--bg-input, var(--bg-primary));
    border: 1px solid var(--border-color);
    border-radius: 4px;
    color: var(--text-secondary);
  }
  .settings-search-wrap:focus-within {
    border-color: var(--border-focus, var(--accent));
  }
  .settings-search-input {
    flex: 1;
    border: none;
    background: transparent;
    outline: none;
    font-size: 12px;
    color: var(--text-primary);
    padding: 4px 0;
    min-width: 0;
  }

  /* The modal body carries no padding for this dialog (see Modal.svelte), so the sidebar divider
     and the footer rule run edge to edge instead of stopping short of the border. Each region
     below pays for its own inset. */
  .settings-layout {
    display: flex;
    height: min(440px, 58vh);
  }

  .settings-tabs {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 10px 8px;
    border-right: 1px solid var(--border-color);
    width: 150px;
    flex-shrink: 0;
    overflow-y: auto;
  }

  .tab-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 6px 10px;
    background: transparent;
    border: none;
    border-radius: 4px;
    color: var(--text-secondary);
    font-size: 12px;
    cursor: pointer;
    text-align: left;
  }
  .tab-item:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }
  .tab-item.active {
    background: var(--accent-muted);
    color: var(--text-bright);
  }
  .tab-item.active :global(svg) {
    color: var(--accent);
  }

  .settings-content {
    flex: 1;
    padding: 16px 18px 20px;
    overflow-y: auto;
    min-width: 300px;
  }

  .settings-loading {
    color: var(--text-secondary);
    font-size: 12px;
    padding: 24px;
    text-align: center;
  }

  .settings-footer {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 12px;
    padding: 10px 16px;
    border-top: 1px solid var(--border-color);
  }
  .settings-footer-actions {
    display: flex;
    justify-content: flex-end;
    gap: 6px;
    margin-left: auto;
  }
  .settings-footer button {
    padding: 4px 14px;
    font-size: 12px;
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
</style>
