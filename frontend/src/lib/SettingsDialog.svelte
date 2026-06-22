<script lang="ts">
  import Modal from './Modal.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import {
    getSettings,
    saveSettings,
    parseHotkeyEvent,
    normalizeHotkey,
    DEFAULT_SESSION_HOTKEYS,
    getAuditSessionState,
    enableAuditSecretLogging,
    disableAuditSecretLogging,
  } from '../stores/api';
  import {
    SETTINGS_TAB_LABELS,
    tabHasSearchMatches,
    shouldShowSettingsSection,
    shouldShowSectionTabLabel,
    type SettingsTabId,
  } from './settingsSearch';
  import {
    applyUiScalePercent,
    DEFAULT_UI_SCALE_PERCENT,
    normalizeUiScalePercent,
    UI_SCALE_PRESETS,
  } from './uiScale';
  import {
    Shield,
    Terminal,
    Palette,
    Info,
    ExternalLink,
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

  let lockoutEnabled = false;
  let lockoutIdleMinutes = 5;
  let lockOnMinimize = false;
  let terminalFontFamily = 'Cascadia Code, Consolas, Courier New, monospace';
  let terminalFontSize = 14;
  let terminalFontColor = '#cccccc';
  let externalEditorPath = '';
  let theme = 'dark';
  let uiScalePercent = DEFAULT_UI_SCALE_PERCENT;
  let uiScaleAtOpen = DEFAULT_UI_SCALE_PERCENT;

  const monoFonts = [
    'Cascadia Code, Consolas, Courier New, monospace',
    'Consolas, Courier New, monospace',
    'Courier New, monospace',
    'Fira Code, monospace',
    'JetBrains Mono, monospace',
    'Source Code Pro, monospace',
    'Ubuntu Mono, monospace',
    'Hack, monospace',
    'Inconsolata, monospace',
    'Menlo, Monaco, monospace',
    'SF Mono, monospace',
    'IBM Plex Mono, monospace',
    'Roboto Mono, monospace',
  ];
  let pingEnabled = true;
  let pingMode = 'interval';
  let pingIntervalSeconds = 5;
  let transferSpeedLimitKbps = 0;
  let connectionTimeoutSeconds = 15;
  let maxConcurrentTransfers = 4;
  let sessionHotkeyCreate = DEFAULT_SESSION_HOTKEYS.create;
  let sessionHotkeyNext = DEFAULT_SESSION_HOTKEYS.next;
  let sessionHotkeyPrev = DEFAULT_SESSION_HOTKEYS.prev;
  let sessionHotkeyClose = DEFAULT_SESSION_HOTKEYS.close;
  let hotkeyConflict = '';

  let auditLogEnabled = false;
  let auditRetentionMode = 'days';
  let auditRetentionDays = 30;
  let auditRetentionCount = 100;
  let auditShowUsername = false;
  let auditShowConnection = false;
  let auditLogSecrets = false;
  let auditSecretsConfirmShow = false;

  const tabs: { id: SettingsTabId; label: string; icon: typeof Shield }[] = [
    { id: 'about', label: 'About', icon: Info },
    { id: 'appearance', label: 'Appearance', icon: Palette },
    { id: 'audit', label: 'Audit Log', icon: FileText },
    { id: 'files', label: 'Files', icon: FileEdit },
    { id: 'hotkeys', label: 'Hotkeys', icon: Keyboard },
    { id: 'network', label: 'Network', icon: Wifi },
    { id: 'security', label: 'Security', icon: Shield },
    { id: 'terminal', label: 'Terminal', icon: Terminal },
  ];

  $: isSearching = searchQuery.trim().length > 0;
  $: searchViewState = { isSearching, activeTab, searchQuery, searchPinnedTab };
  $: visibleTabs = isSearching
    ? tabs.filter((tab) => tabHasSearchMatches(tab.id, searchQuery))
    : tabs;
  $: showSaveButton = !(!isSearching && activeTab === 'about');

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

  function sectionTabLabel(tabId: SettingsTabId, sectionId: string): boolean {
    return shouldShowSectionTabLabel(tabId, sectionId, searchViewState);
  }

  function handleTabClick(tabId: SettingsTabId) {
    if (isSearching) {
      if (searchPinnedTab === tabId) {
        searchPinnedTab = null;
      } else {
        searchPinnedTab = tabId;
      }
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
    const s = await getSettings();
    if (s) {
      lockoutEnabled = s.lockoutEnabled ?? false;
      lockoutIdleMinutes = s.lockoutIdleMinutes ?? 5;
      lockOnMinimize = s.lockOnMinimize ?? false;
      terminalFontFamily = s.terminalFontFamily || 'Cascadia Code, Consolas, Courier New, monospace';
      terminalFontSize = s.terminalFontSize || 14;
      terminalFontColor = s.terminalFontColor || '#cccccc';
      externalEditorPath = s.externalEditorPath || '';
      theme = s.theme || 'dark';
      uiScalePercent = s.uiScalePercent ?? DEFAULT_UI_SCALE_PERCENT;
      uiScaleAtOpen = uiScalePercent;
      pingEnabled = s.pingEnabled ?? true;
      pingMode = s.pingMode ?? 'interval';
      pingIntervalSeconds = s.pingIntervalSeconds ?? 5;
      transferSpeedLimitKbps = s.transferSpeedLimitKbps ?? 0;
      connectionTimeoutSeconds = s.connectionTimeoutSeconds ?? 15;
      maxConcurrentTransfers = s.maxConcurrentTransfers ?? 4;
      sessionHotkeyCreate = normalizeHotkey(s.sessionHotkeyCreate || DEFAULT_SESSION_HOTKEYS.create);
      sessionHotkeyNext = normalizeHotkey(s.sessionHotkeyNext || DEFAULT_SESSION_HOTKEYS.next);
      sessionHotkeyPrev = normalizeHotkey(s.sessionHotkeyPrev || DEFAULT_SESSION_HOTKEYS.prev);
      sessionHotkeyClose = normalizeHotkey(s.sessionHotkeyClose || DEFAULT_SESSION_HOTKEYS.close);
      auditLogEnabled = s.auditLogEnabled ?? false;
      auditRetentionMode = s.auditRetentionMode ?? 'days';
      auditRetentionDays = s.auditRetentionDays ?? 30;
      auditRetentionCount = s.auditRetentionCount ?? 100;
      auditShowUsername = s.auditShowUsername ?? false;
      auditShowConnection = s.auditShowConnection ?? false;
    }
    const sessionState = await getAuditSessionState();
    auditLogSecrets = sessionState?.logSecretsEnabled ?? false;
    hotkeyConflict = '';
    loading = false;
  }

  async function handleAuditSecretsToggle(e: Event) {
    const checked = (e.target as HTMLInputElement).checked;
    if (checked) {
      (e.target as HTMLInputElement).checked = false;
      auditSecretsConfirmShow = true;
      return;
    }
    auditLogSecrets = false;
    disableAuditSecretLogging();
  }

  async function confirmAuditSecrets() {
    auditSecretsConfirmShow = false;
    const ok = await enableAuditSecretLogging(true);
    if (ok) auditLogSecrets = true;
  }

  function cancelAuditSecrets() {
    auditSecretsConfirmShow = false;
    auditLogSecrets = false;
  }

  function validateHotkeyConflicts(): string {
    const entries = [
      { id: 'create', label: 'Create session', value: normalizeHotkey(sessionHotkeyCreate) },
      { id: 'next', label: 'Next session', value: normalizeHotkey(sessionHotkeyNext) },
      { id: 'prev', label: 'Previous session', value: normalizeHotkey(sessionHotkeyPrev) },
      { id: 'close', label: 'Close session', value: normalizeHotkey(sessionHotkeyClose) },
    ];
    for (let i = 0; i < entries.length; i++) {
      for (let j = i + 1; j < entries.length; j++) {
        if (entries[i].value && entries[i].value === entries[j].value) {
          return `${entries[i].label} conflicts with ${entries[j].label}`;
        }
      }
    }
    return '';
  }

  function captureHotkey(e: KeyboardEvent, field: 'create' | 'next' | 'prev' | 'close') {
    e.preventDefault();
    e.stopPropagation();
    const key = parseHotkeyEvent(e);
    if (!key) return;
    if (field === 'create') sessionHotkeyCreate = key;
    if (field === 'next') sessionHotkeyNext = key;
    if (field === 'prev') sessionHotkeyPrev = key;
    if (field === 'close') sessionHotkeyClose = key;
    hotkeyConflict = validateHotkeyConflicts();
  }

  function resetHotkeysToDefault() {
    sessionHotkeyCreate = DEFAULT_SESSION_HOTKEYS.create;
    sessionHotkeyNext = DEFAULT_SESSION_HOTKEYS.next;
    sessionHotkeyPrev = DEFAULT_SESSION_HOTKEYS.prev;
    sessionHotkeyClose = DEFAULT_SESSION_HOTKEYS.close;
    hotkeyConflict = '';
  }

  function handleUiScaleChange() {
    uiScalePercent = normalizeUiScalePercent(Number(uiScalePercent));
    applyUiScalePercent(uiScalePercent);
  }

  function closeSettings() {
    applyUiScalePercent(uiScaleAtOpen);
    show = false;
  }

  async function handleSave() {
    hotkeyConflict = validateHotkeyConflicts();
    if (hotkeyConflict) return;
    saving = true;
    await saveSettings({
      lockoutEnabled,
      lockoutIdleMinutes,
      lockOnMinimize,
      terminalFontFamily,
      terminalFontSize,
      terminalFontColor,
      externalEditorPath,
      theme,
      uiScalePercent,
      pingEnabled,
      pingMode,
      pingIntervalSeconds,
      transferSpeedLimitKbps,
      connectionTimeoutSeconds,
      maxConcurrentTransfers,
      sessionHotkeyCreate: normalizeHotkey(sessionHotkeyCreate),
      sessionHotkeyNext: normalizeHotkey(sessionHotkeyNext),
      sessionHotkeyPrev: normalizeHotkey(sessionHotkeyPrev),
      sessionHotkeyClose: normalizeHotkey(sessionHotkeyClose),
      auditLogEnabled,
      auditRetentionMode,
      auditRetentionDays,
      auditRetentionCount,
      auditShowUsername,
      auditShowConnection,
    });
    window.dispatchEvent(new CustomEvent('app-settings-updated'));
    uiScaleAtOpen = uiScalePercent;
    saving = false;
    show = false;
  }

  const appVersion = '2.0.0';
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
          {#if isSearching ? shouldShowSettingsSection('about', 'info', searchViewState) : activeTab === 'about'}
            {#if sectionTabLabel('about', 'info')}
              <div class="section-tab-label">{SETTINGS_TAB_LABELS.about}</div>
            {/if}
            <div class="section">
              <h4>SSH Client</h4>
              <p class="version-text">Version {appVersion}</p>
              <div class="about-links">
                <button class="secondary about-link" on:click={() => window.open('https://github.com/teoritty/xQuakShell/releases/', '_blank')}>
                  <ExternalLink size={13} />
                  Check for Updates
                </button>
                <button class="secondary about-link" on:click={() => window.open('https://github.com/teoritty/xQuakShell/issues/new', '_blank')}>
                  <ExternalLink size={13} />
                  Report an Issue
                </button>
              </div>
            </div>
          {/if}

          {#if isSearching ? shouldShowSettingsSection('appearance', 'theme', searchViewState) : activeTab === 'appearance'}
            {#if sectionTabLabel('appearance', 'theme')}
              <div class="section-tab-label">{SETTINGS_TAB_LABELS.appearance}</div>
            {/if}
            <div class="section">
              <h4>Theme</h4>
              <div class="theme-options">
                <label class="theme-option" class:selected={theme === 'dark'}>
                  <input type="radio" bind:group={theme} value="dark" />
                  <div class="theme-swatch dark-swatch"></div>
                  <span>Dark</span>
                </label>
                <!-- <label class="theme-option" class:selected={theme === 'light'} title="Coming soon">
                  <input type="radio" bind:group={theme} value="light" disabled />
                  <div class="theme-swatch light-swatch"></div>
                  <span>Light (soon)</span>
                </label> -->
              </div>
            </div>
          {/if}

          {#if isSearching ? shouldShowSettingsSection('appearance', 'scale', searchViewState) : activeTab === 'appearance'}
            {#if sectionTabLabel('appearance', 'scale')}
              <div class="section-tab-label">{SETTINGS_TAB_LABELS.appearance}</div>
            {/if}
            <div class="section">
              <h4>Interface scale</h4>
              <p class="section-desc">Adjust the size of the entire interface. Layout reflows to fit the window — nothing is cropped like browser zoom.</p>
              <label class="field-block">
                <span>Scale</span>
                <select bind:value={uiScalePercent} on:change={handleUiScaleChange}>
                  {#each UI_SCALE_PRESETS as preset}
                    <option value={preset}>{preset}%</option>
                  {/each}
                </select>
              </label>
            </div>
          {/if}

          {#if isSearching ? shouldShowSettingsSection('audit', 'general', searchViewState) : activeTab === 'audit'}
            {#if sectionTabLabel('audit', 'general')}
              <div class="section-tab-label">{SETTINGS_TAB_LABELS.audit}</div>
            {/if}
            <div class="section">
              <h4>General</h4>
              <p class="section-desc">Command audit logging is disabled by default. When enabled, submitted commands are stored locally on Enter.</p>
              <label class="checkbox-row">
                <input type="checkbox" bind:checked={auditLogEnabled} />
                Enable audit log
              </label>
            </div>
          {/if}

          {#if isSearching ? shouldShowSettingsSection('audit', 'retention', searchViewState) : activeTab === 'audit'}
            {#if sectionTabLabel('audit', 'retention')}
              <div class="section-tab-label">{SETTINGS_TAB_LABELS.audit}</div>
            {/if}
            <div class="section">
              <h4>Retention</h4>
              <p class="section-desc">Old entries are deleted automatically and cannot be recovered.</p>
              <label class="checkbox-row">
                <input type="radio" bind:group={auditRetentionMode} value="days" disabled={!auditLogEnabled} />
                By time (days)
              </label>
              <label class="field-inline">
                <span>Keep entries for (days)</span>
                <input type="number" bind:value={auditRetentionDays} min="1" max="365" disabled={!auditLogEnabled || auditRetentionMode !== 'days'} />
              </label>
              <label class="checkbox-row">
                <input type="radio" bind:group={auditRetentionMode} value="count" disabled={!auditLogEnabled} />
                By count
              </label>
              <label class="field-inline">
                <span>Maximum entries</span>
                <input type="number" bind:value={auditRetentionCount} min="10" max="10000" disabled={!auditLogEnabled || auditRetentionMode !== 'count'} />
              </label>
            </div>
          {/if}

          {#if isSearching ? shouldShowSettingsSection('audit', 'privacy', searchViewState) : activeTab === 'audit'}
            {#if sectionTabLabel('audit', 'privacy')}
              <div class="section-tab-label">{SETTINGS_TAB_LABELS.audit}</div>
            {/if}
            <div class="section">
              <h4>Privacy</h4>
              <p class="section-desc">When enabled, metadata is stored in the audit log and shown in the viewer.</p>
              <label class="checkbox-row">
                <input type="checkbox" bind:checked={auditShowUsername} disabled={!auditLogEnabled} />
                Log &amp; show username
              </label>
              <label class="checkbox-row">
                <input type="checkbox" bind:checked={auditShowConnection} disabled={!auditLogEnabled} />
                Log &amp; show connection (name and host)
              </label>
            </div>
          {/if}

          {#if isSearching ? shouldShowSettingsSection('audit', 'secrets', searchViewState) : activeTab === 'audit'}
            {#if sectionTabLabel('audit', 'secrets')}
              <div class="section-tab-label">{SETTINGS_TAB_LABELS.audit}</div>
            {/if}
            <div class="section">
              <h4>Sensitive data (session only)</h4>
              <p class="section-desc">When enabled, passwords and secrets are logged in plaintext until you lock the vault or restart the app. This setting is not saved to the vault.</p>
              <label class="checkbox-row">
                <input type="checkbox" checked={auditLogSecrets} on:change={handleAuditSecretsToggle} disabled={!auditLogEnabled} />
                Log secrets this session
              </label>
            </div>
          {/if}

          {#if isSearching ? shouldShowSettingsSection('files', 'editor', searchViewState) : activeTab === 'files'}
            {#if sectionTabLabel('files', 'editor')}
              <div class="section-tab-label">{SETTINGS_TAB_LABELS.files}</div>
            {/if}
            <div class="section">
              <h4>External Editor (Edit on the fly)</h4>
              <p class="section-desc">Path to editor executable for editing remote files. When you edit a remote file, it is downloaded, opened with this editor, and changes are automatically re-uploaded on save.</p>
              <label class="field-block">
                <span>Editor path</span>
                <input type="text" bind:value={externalEditorPath} placeholder="e.g. code, notepad.exe, C:\...\gvim.exe" />
              </label>
            </div>
          {/if}

          {#if isSearching ? shouldShowSettingsSection('hotkeys', 'session', searchViewState) : activeTab === 'hotkeys'}
            {#if sectionTabLabel('hotkeys', 'session')}
              <div class="section-tab-label">{SETTINGS_TAB_LABELS.hotkeys}</div>
            {/if}
            <div class="section">
              <h4>Session Hotkeys</h4>
              <p class="section-desc">Configure global shortcuts for session actions. Click a field and press a key combination.</p>
              <label class="field-block">
                <span>Create session</span>
                <input class="hotkey-input" type="text" bind:value={sessionHotkeyCreate} on:keydown={(e) => captureHotkey(e, 'create')} />
              </label>
              <label class="field-block">
                <span>Next session tab</span>
                <input class="hotkey-input" type="text" bind:value={sessionHotkeyNext} on:keydown={(e) => captureHotkey(e, 'next')} />
              </label>
              <label class="field-block">
                <span>Previous session tab</span>
                <input class="hotkey-input" type="text" bind:value={sessionHotkeyPrev} on:keydown={(e) => captureHotkey(e, 'prev')} />
              </label>
              <label class="field-block">
                <span>Close active session</span>
                <input class="hotkey-input" type="text" bind:value={sessionHotkeyClose} on:keydown={(e) => captureHotkey(e, 'close')} />
              </label>
              {#if hotkeyConflict}
                <div class="hotkey-conflict">{hotkeyConflict}</div>
              {/if}
              <div class="hotkey-actions">
                <button class="secondary" type="button" on:click={resetHotkeysToDefault}>Reset to defaults</button>
              </div>
            </div>
          {/if}

          {#if isSearching ? shouldShowSettingsSection('network', 'ping', searchViewState) : activeTab === 'network'}
            {#if sectionTabLabel('network', 'ping')}
              <div class="section-tab-label">{SETTINGS_TAB_LABELS.network}</div>
            {/if}
            <div class="section">
              <h4>Connection Ping</h4>
              <p class="section-desc">Check host reachability via TCP connect.</p>
              <label class="checkbox-row">
                <input type="checkbox" bind:checked={pingEnabled} />
                Enable automatic ping
              </label>
              <label class="field-block">
                <span>Ping mode</span>
                <select bind:value={pingMode} disabled={!pingEnabled}>
                  <option value="on_change">On connection settings change only</option>
                  <option value="interval">Every N seconds</option>
                </select>
              </label>
              <label class="field-inline">
                <span>Ping interval (seconds)</span>
                <input
                  type="number"
                  bind:value={pingIntervalSeconds}
                  min="5"
                  max="300"
                  disabled={pingMode !== 'interval' || !pingEnabled}
                />
              </label>
            </div>
          {/if}

          {#if isSearching ? shouldShowSettingsSection('network', 'transfer', searchViewState) : activeTab === 'network'}
            {#if sectionTabLabel('network', 'transfer')}
              <div class="section-tab-label">{SETTINGS_TAB_LABELS.network}</div>
            {/if}
            <div class="section">
              <h4>File Transfer</h4>
              <p class="section-desc">Settings for sysadmins and DevOps.</p>
              <label class="field-block">
                <span>Speed limit (Kbps)</span>
                <input type="number" bind:value={transferSpeedLimitKbps} min="0" placeholder="0 = unlimited" />
              </label>
              <label class="field-block">
                <span>Connection timeout (seconds)</span>
                <input type="number" bind:value={connectionTimeoutSeconds} min="5" max="300" />
              </label>
              <label class="field-block">
                <span>Max concurrent transfers</span>
                <input type="number" bind:value={maxConcurrentTransfers} min="1" max="16" />
              </label>
            </div>
          {/if}

          {#if isSearching ? shouldShowSettingsSection('security', 'lockout', searchViewState) : activeTab === 'security'}
            {#if sectionTabLabel('security', 'lockout')}
              <div class="section-tab-label">{SETTINGS_TAB_LABELS.security}</div>
            {/if}
            <div class="section">
              <h4>Session Lockout</h4>
              <p class="section-desc">Automatically lock the vault after inactivity.</p>
              <label class="checkbox-row">
                <input type="checkbox" bind:checked={lockoutEnabled} />
                Enable lockout on idle timeout
              </label>
              <label class="field-inline">
                <span>Idle timeout (minutes)</span>
                <input
                  type="number"
                  bind:value={lockoutIdleMinutes}
                  min="1"
                  max="120"
                  disabled={!lockoutEnabled}
                />
              </label>
              <label class="checkbox-row">
                <input type="checkbox" bind:checked={lockOnMinimize} />
                Lock when application is minimized
              </label>
            </div>
          {/if}

          {#if isSearching ? shouldShowSettingsSection('terminal', 'font', searchViewState) : activeTab === 'terminal'}
            {#if sectionTabLabel('terminal', 'font')}
              <div class="section-tab-label">{SETTINGS_TAB_LABELS.terminal}</div>
            {/if}
            <div class="section">
              <h4>Terminal Font</h4>
              <label class="field-block">
                <span>Font Family</span>
                <select bind:value={terminalFontFamily}>
                  {#each monoFonts as font}
                    <option value={font} style="font-family: {font}">{font.split(',')[0].trim()}</option>
                  {/each}
                </select>
              </label>
              <label class="field-block">
                <span>Font Size (px)</span>
                <input type="number" bind:value={terminalFontSize} min="8" max="32" />
              </label>
              <label class="field-block">
                <span>Font Color</span>
                <div class="color-picker-row">
                  <input type="color" bind:value={terminalFontColor} class="color-input" />
                  <input type="text" bind:value={terminalFontColor} class="color-hex" placeholder="#cccccc" />
                </div>
              </label>
              <div class="font-preview" style="font-family: {terminalFontFamily}; font-size: calc({terminalFontSize}px * var(--ui-scale)); color: {terminalFontColor};">
                user@server:~$ ls -la
              </div>
            </div>
          {/if}
        {/if}
      </div>
    </div>

    <div class="settings-footer">
      <button class="secondary" on:click={closeSettings}>Cancel</button>
      {#if showSaveButton}
        <button class="primary" on:click={handleSave} disabled={saving}>
          <Save size={13} />
          {saving ? 'Saving...' : 'Save'}
        </button>
      {/if}
    </div>
  </Modal>
{/if}

<ConfirmDialog
  show={auditSecretsConfirmShow}
  title="Enable secret logging"
  message="Secrets will be stored in plaintext in the local audit database. This applies only until you lock the vault or restart the app."
  critical={true}
  requireCheckbox={true}
  checkboxLabel="I understand that sensitive data will be logged in plaintext"
  confirmLabel="Enable"
  cancelLabel="Cancel"
  on:confirm={confirmAuditSecrets}
  on:cancel={cancelAuditSecrets}
/>

<style>
  .settings-search-wrap {
    display: flex;
    align-items: center;
    gap: 6px;
    width: 100%;
    padding: 0 8px;
    background: var(--bg-input, var(--bg-primary));
    border: 1px solid var(--border-color);
    border-radius: 3px;
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

  .settings-layout {
    display: flex;
    gap: 0;
    height: 420px;
  }

  .settings-tabs {
    display: flex;
    flex-direction: column;
    gap: 1px;
    border-right: 1px solid var(--border-color);
    width: 130px;
    flex-shrink: 0;
    overflow-y: auto;
  }

  .tab-item {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 8px 12px;
    background: transparent;
    border: none;
    color: var(--text-secondary);
    font-size: 12px;
    cursor: pointer;
    text-align: left;
    border-radius: 0;
  }
  .tab-item:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }
  .tab-item.active {
    background: var(--bg-active);
    color: var(--text-bright);
  }

  .settings-content {
    flex: 1;
    padding: 12px 16px;
    overflow-y: auto;
    min-width: 300px;
  }

  .settings-loading {
    color: var(--text-secondary);
    font-size: 12px;
    padding: 20px;
    text-align: center;
  }

  .section-tab-label {
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--accent);
    margin-bottom: 6px;
  }

  .section-tab-label:not(:first-child) {
    margin-top: 4px;
  }

  .section h4 {
    font-size: 13px;
    font-weight: 600;
    color: var(--text-bright);
    margin: 0 0 4px;
  }

  .section-desc {
    font-size: 11px;
    color: var(--text-secondary);
    margin: 0 0 12px;
  }

  .section {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .section + .section,
  .section-tab-label:not(:first-child) + .section {
    border-top: 1px solid var(--border-color);
    padding-top: 14px;
    margin-top: 14px;
  }

  .checkbox-row {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--text-primary);
    cursor: pointer;
  }
  .checkbox-row input[type="checkbox"],
  .checkbox-row input[type="radio"] {
    margin: 0;
    width: 14px;
    height: 14px;
  }

  .field-inline {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 12px;
    color: var(--text-primary);
  }
  .field-inline input[type="number"] {
    width: 60px;
  }

  .field-block {
    display: flex;
    flex-direction: column;
    gap: 3px;
    font-size: 12px;
    color: var(--text-primary);
  }
  .field-block span {
    font-size: 11px;
    color: var(--text-secondary);
  }

  .color-picker-row {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .color-input {
    width: 36px;
    height: 28px;
    padding: 0;
    border: 1px solid var(--border-color);
    border-radius: 3px;
    cursor: pointer;
    background: transparent;
  }
  .color-input::-webkit-color-swatch-wrapper { padding: 2px; }
  .color-input::-webkit-color-swatch { border-radius: 2px; border: none; }
  .color-hex {
    width: 80px;
    font-size: 12px;
    font-family: var(--font-mono, monospace);
  }

  .font-preview {
    padding: 10px;
    background: #1e1e1e;
    border: 1px solid var(--border-color);
    border-radius: 2px;
    line-height: 1.4;
    margin-top: 4px;
  }

  .theme-options {
    display: flex;
    gap: 12px;
  }

  .theme-option {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 4px;
    cursor: pointer;
    padding: 8px;
    border: 1px solid var(--border-color);
    border-radius: 4px;
    font-size: 11px;
    color: var(--text-secondary);
  }
  .theme-option.selected {
    border-color: var(--accent);
    color: var(--text-primary);
  }
  .theme-option input { display: none; }

  .theme-swatch {
    width: 48px;
    height: 32px;
    border-radius: 3px;
    border: 1px solid var(--border-color);
  }
  .dark-swatch { background: #1e1e1e; }
  .light-swatch { background: #f5f5f5; }

  .version-text {
    font-size: 12px;
    color: var(--text-secondary);
    margin: 0;
  }

  .about-links {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-top: 8px;
  }
  .about-link {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    padding: 6px 12px;
    width: fit-content;
  }

  .settings-footer {
    display: flex;
    justify-content: flex-end;
    gap: 6px;
    padding-top: 12px;
    border-top: 1px solid var(--border-color);
    margin-top: 12px;
  }
  .settings-footer button {
    padding: 5px 14px;
    font-size: 12px;
    display: inline-flex;
    align-items: center;
    gap: 4px;
  }

  .hotkey-input {
    font-family: var(--font-mono, monospace);
  }

  .hotkey-conflict {
    font-size: 11px;
    color: var(--danger, #f44747);
  }

  .hotkey-actions {
    display: flex;
    justify-content: flex-start;
    margin-top: 2px;
  }
</style>
