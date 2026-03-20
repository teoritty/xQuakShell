<script lang="ts">
  import Modal from './Modal.svelte';
  import { getSettings, saveSettings, parseHotkeyEvent, normalizeHotkey, DEFAULT_SESSION_HOTKEYS } from '../stores/api';
  import { Shield, Terminal, Palette, Info, ExternalLink, Save, Wifi, FileEdit, Gauge, Keyboard } from 'lucide-svelte';

  export let show = false;

  let activeTab = 'security';
  let loading = true;
  let saving = false;

  let lockoutEnabled = false;
  let lockoutIdleMinutes = 5;
  let lockOnMinimize = false;
  let terminalFontFamily = 'Cascadia Code, Consolas, Courier New, monospace';
  let terminalFontSize = 14;
  let terminalFontColor = '#cccccc';
  let externalEditorPath = '';
  let theme = 'dark';

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

  const tabs = [
    { id: 'security', label: 'Security', icon: Shield },
    { id: 'terminal', label: 'Terminal', icon: Terminal },
    { id: 'files', label: 'Files', icon: FileEdit },
    { id: 'network', label: 'Network', icon: Wifi },
    { id: 'hotkeys', label: 'Hotkeys', icon: Keyboard },
    { id: 'appearance', label: 'Appearance', icon: Palette },
    { id: 'about', label: 'About', icon: Info },
  ];

  $: if (show) loadSettings();

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
    }
    hotkeyConflict = '';
    loading = false;
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
    });
    window.dispatchEvent(new CustomEvent('app-settings-updated'));
    saving = false;
    show = false;
  }

  const appVersion = '2.0.0';
</script>

{#if show}
  <Modal title="Settings" show={true} on:close={() => show = false}>
    <div class="settings-layout">
      <div class="settings-tabs">
        {#each tabs as tab}
          <button
            class="tab-item"
            class:active={activeTab === tab.id}
            on:click={() => activeTab = tab.id}
          >
            <svelte:component this={tab.icon} size={14} />
            {tab.label}
          </button>
        {/each}
      </div>

      <div class="settings-content">
        {#if loading}
          <div class="settings-loading">Loading...</div>
        {:else if activeTab === 'security'}
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

        {:else if activeTab === 'terminal'}
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

            <div class="font-preview" style="font-family: {terminalFontFamily}; font-size: {terminalFontSize}px; color: {terminalFontColor};">
              user@server:~$ ls -la
            </div>
          </div>

        {:else if activeTab === 'files'}
          <div class="section">
            <h4>External Editor (Edit on the fly)</h4>
            <p class="section-desc">Path to editor executable for editing remote files. When you edit a remote file, it is downloaded, opened with this editor, and changes are automatically re-uploaded on save.</p>
            <label class="field-block">
              <span>Editor path</span>
              <input type="text" bind:value={externalEditorPath} placeholder="e.g. code, notepad.exe, C:\...\gvim.exe" />
            </label>
          </div>

        {:else if activeTab === 'network'}
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

            <h4 style="margin-top: 20px;">File Transfer</h4>
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

        {:else if activeTab === 'hotkeys'}
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

        {:else if activeTab === 'appearance'}
          <div class="section">
            <h4>Theme</h4>
            <div class="theme-options">
              <label class="theme-option" class:selected={theme === 'dark'}>
                <input type="radio" bind:group={theme} value="dark" />
                <div class="theme-swatch dark-swatch"></div>
                <span>Dark</span>
              </label>
              <label class="theme-option" class:selected={theme === 'light'} title="Coming soon">
                <input type="radio" bind:group={theme} value="light" disabled />
                <div class="theme-swatch light-swatch"></div>
                <span>Light (soon)</span>
              </label>
            </div>
          </div>

        {:else if activeTab === 'about'}
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
      </div>
    </div>

    <div class="settings-footer">
      <button class="secondary" on:click={() => show = false}>Cancel</button>
      {#if activeTab !== 'about'}
        <button class="primary" on:click={handleSave} disabled={saving}>
          <Save size={13} />
          {saving ? 'Saving...' : 'Save'}
        </button>
      {/if}
    </div>
  </Modal>
{/if}

<style>
  .settings-layout {
    display: flex;
    gap: 0;
    min-height: 280px;
  }

  .settings-tabs {
    display: flex;
    flex-direction: column;
    gap: 1px;
    padding: 0 0 0 0;
    border-right: 1px solid var(--border-color);
    width: 130px;
    flex-shrink: 0;
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

  .section h4 {
    font-size: 13px;
    font-weight: 600;
    color: var(--text-bright);
    margin-bottom: 4px;
  }

  .section-desc {
    font-size: 11px;
    color: var(--text-secondary);
    margin-bottom: 12px;
  }

  .section {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .checkbox-row {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: 12px;
    color: var(--text-primary);
    cursor: pointer;
  }
  .checkbox-row input[type="checkbox"] {
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
