<script lang="ts">
  import { onMount } from 'svelte';
  import Sidebar from './lib/Sidebar.svelte';
  import PluginDialog from './lib/PluginDialog.svelte';
  import TileGrid from './lib/tiles/TileGrid.svelte';
  import VaultUnlock from './lib/VaultUnlock.svelte';
  import UpdateBanner from './lib/UpdateBanner.svelte';
  import KnownHostsManager from './lib/KnownHostsManager.svelte';
  import KeyManager from './lib/KeyManager.svelte';
  import HostKeyDialog from './lib/HostKeyDialog.svelte';
  import PeerTrustPrompt from './lib/PeerTrustPrompt.svelte';
  import SessionPassphrasePrompt from './lib/SessionPassphrasePrompt.svelte';
  import PeerTrustManager from './lib/PeerTrustManager.svelte';
  import AuditLogView from './lib/AuditLogView.svelte';
  import ErrorDialog from './lib/ErrorDialog.svelte';
  import RecoveryKeyDialog from './lib/vault/RecoveryKeyDialog.svelte';
  import ConflictDialog from './lib/ConflictDialog.svelte';
  import SettingsDialog from './lib/SettingsDialog.svelte';
  import PluginsDialog from './lib/plugins/PluginsDialog.svelte';
  import TopBarActions from './lib/TopBarActions.svelte';
  import type { SettingsTabId } from './lib/settingsSearch';
  import ScriptsDialog from './lib/ScriptsDialog.svelte';
  import PluginCommandPalette from './lib/PluginCommandPalette.svelte';
  import { pluginContributions, initPluginContributionEvents, initPluginViewMessageEvents, refreshPluginContributions } from './stores/pluginState';
  import { sessions, activeTabId, vaultUnlocked, pendingHostKey, pendingRecoveryKey } from './stores/appState';
  import { subscribeToEvents } from './events/subscribe';
  import { resolveHostKeyRpc as resolveHostKey } from './api/sessions';
  import { createSessionFromSelection } from './actions/sessionActions';
  import { startAutoReconnect, refreshAutoReconnectSetting } from './actions/reconnectActions';
  // The close and cycle hotkeys address whatever the tab bar shows — an SSH session or a plugin
  // surface (ADR-015) — so they route through the tab layer, not the session one.
  import { focusNextTab, focusPrevTab, closeActiveTab } from './actions/tabActions';
  import { getSettings, applyAppearanceSettings } from './actions/settingsActions';
  import { parseHotkeyEvent } from './hotkeys/hotkeys';
  import WelcomeScreen from './lib/WelcomeScreen.svelte';
  import { DEFAULT_APP_HOTKEYS, hotkeysFromSettings } from './hotkeys/appHotkeys';
  import MultiInputBar from './lib/terminalTools/MultiInputBar.svelte';
  import { toggleMultiInputPicker, stopMultiInputIfRunning } from './stores/terminalTools';
  import { openTerminalSearch } from './stores/terminalSearch';
  import { openLocalTerminal } from './actions/localTerminalActions';
  import { hasOpenTabs } from './stores/surfaceState';

  let showKnownHosts = false;
  let showPeerTrust = false;
  let showKeyManager = false;
  let showAuditLog = false;
  let showSettings = false;
  let showPlugins = false;
  let settingsInitialTab: SettingsTabId = 'about';
  let showScripts = false;
  let commandPalette: PluginCommandPalette;

  function openSettings(tab: SettingsTabId = 'about') {
    settingsInitialTab = tab;
    showSettings = true;
  }

  function openSettingsFromAudit(tab?: string) {
    openSettings((tab as SettingsTabId) || 'audit');
    showAuditLog = false;
  }

  let hotkeys = DEFAULT_APP_HOTKEYS;

  $: showHostKeyDialog = $pendingHostKey !== null;
  $: hostKeyHost = $pendingHostKey?.host ?? '';
  $: hostKeyType = $pendingHostKey?.keyType ?? '';
  $: hostKeyFingerprint = $pendingHostKey?.fingerprint ?? '';
  $: hostKeyKeyBase64 = $pendingHostKey?.keyBase64 ?? '';
  $: hostKeyMismatch = $pendingHostKey?.mismatch ?? false;
  $: hostKeySessionId = $pendingHostKey?.sessionId ?? '';

  async function handleHostKeyAccept() {
    if (!hostKeySessionId || !hostKeyKeyBase64) return;
    const action = hostKeyMismatch ? 'replace' : 'add';
    await resolveHostKey(hostKeySessionId, action);
    pendingHostKey.set(null);
  }

  function handleHostKeyCancel() {
    pendingHostKey.set(null);
  }

  function getApp() { return (window as any).go?.main?.App; }

  function reportActivity() {
    const app = getApp();
    if (app) app.ReportActivity();
  }

  function shouldIgnoreHotkey(e: KeyboardEvent): boolean {
    const target = e.target as HTMLElement | null;
    if (!target) return false;
    // Important: terminal keeps focus on an internal DIV/canvas wrapper.
    // We intentionally DO NOT ignore shortcuts there, otherwise Ctrl+Shift+Tab / Ctrl+Shift+Q
    // falls through to the remote shell and prints control-sequence garbage.
    if (target.closest('.terminal-container')) return false;
    const tag = target.tagName;
    return (
      target.isContentEditable ||
      tag === 'INPUT' ||
      tag === 'TEXTAREA' ||
      tag === 'SELECT'
    );
  }

  async function loadHotkeysFromSettings() {
    const s = await getSettings();
    if (!s) return;
    hotkeys = hotkeysFromSettings(s);
  }

  onMount(() => {
    subscribeToEvents();
    const stopReconnectWatcher = startAutoReconnect();
    initPluginContributionEvents();
    initPluginViewMessageEvents();
    void refreshPluginContributions();
    if ($vaultUnlocked) {
      loadHotkeysFromSettings();
      void applyAppearanceSettings();
    }

    document.addEventListener('click', reportActivity);
    document.addEventListener('keydown', reportActivity);
    const hotkeyHandler = async (e: KeyboardEvent) => {
      const combo = parseHotkeyEvent(e);
      const ignored = shouldIgnoreHotkey(e);
      if (ignored) return;
      if (!combo) return;
      if (combo === hotkeys.create) {
        e.preventDefault();
        e.stopPropagation();
        await createSessionFromSelection();
        return;
      }
      if (combo === hotkeys.next) {
        e.preventDefault();
        e.stopPropagation();
        focusNextTab();
        return;
      }
      if (combo === hotkeys.prev) {
        e.preventDefault();
        e.stopPropagation();
        focusPrevTab();
        return;
      }
      if (combo === hotkeys.close) {
        e.preventDefault();
        e.stopPropagation();
        await closeActiveTab();
        return;
      }
      if (combo === hotkeys.localTerminal) {
        e.preventDefault();
        e.stopPropagation();
        await openLocalTerminal();
        return;
      }
      if (combo === hotkeys.search) {
        e.preventDefault();
        e.stopPropagation();
        openTerminalSearch();
        return;
      }
      if (combo === hotkeys.multiInputStop && stopMultiInputIfRunning()) {
        e.preventDefault();
        e.stopPropagation();
        return;
      }
      if (combo === hotkeys.multiInput) {
        e.preventDefault();
        e.stopPropagation();
        toggleMultiInputPicker();
        return;
      }
      if (combo === 'Ctrl+Shift+P') {
        e.preventDefault();
        e.stopPropagation();
        commandPalette?.openPalette();
      }
    };
    window.addEventListener('keydown', hotkeyHandler, true);
    const settingsChanged = () => {
      loadHotkeysFromSettings();
      void applyAppearanceSettings();
      void refreshAutoReconnectSetting();
    };
    window.addEventListener('app-settings-updated', settingsChanged as EventListener);

    document.addEventListener('visibilitychange', () => {
      const app = getApp();
      if (!app) return;
      if (document.visibilityState === 'hidden') app.ReportMinimized();
      else app.ReportRestored();
    });
    return () => {
      document.removeEventListener('click', reportActivity);
      document.removeEventListener('keydown', reportActivity);
      window.removeEventListener('keydown', hotkeyHandler, true);
      window.removeEventListener('app-settings-updated', settingsChanged as EventListener);
      stopReconnectWatcher();
    };
  });

  $: statusBarItems = [...($pluginContributions.statusBar || [])].sort(
    (a, b) => (a.priority ?? 0) - (b.priority ?? 0)
  );

  $: if ($vaultUnlocked) {
    loadHotkeysFromSettings();
    void applyAppearanceSettings();
  }

</script>

{#if !$vaultUnlocked}
  <VaultUnlock />
{:else}
  <div class="app-shell">
  <UpdateBanner />
  <div class="app-layout">
    <Sidebar />
    <div class="main-area">
      <div class="top-bar">
        <div class="top-bar-spacer">
          {#if $hasOpenTabs}<MultiInputBar hotkey={hotkeys.multiInput} stopHotkey={hotkeys.multiInputStop} />{/if}
        </div>
        <TopBarActions
          on:scripts={() => (showScripts = true)}
          on:audit={() => (showAuditLog = true)}
          on:knownHosts={() => (showKnownHosts = true)}
          on:peerTrust={() => (showPeerTrust = true)}
          on:keys={() => (showKeyManager = true)}
          on:plugins={() => (showPlugins = true)}
          on:settings={() => openSettings()}
        />
      </div>
      <div class="session-area">
        {#if !$hasOpenTabs}
          <WelcomeScreen {hotkeys} on:settings={() => openSettings()} />
        {:else}
          <TileGrid />
        {/if}
      </div>
    </div>
  </div>

  <!-- Mounted once, above everything: a plugin's modal is not owned by any one tab or panel. -->
  <PluginDialog />

  {#if statusBarItems.length > 0}
    <div class="plugin-status-bar">
      {#each statusBarItems as item (item.pluginId + '.' + item.id)}
        <span class="status-item" title={item.tooltip || item.text}>{item.text}</span>
      {/each}
    </div>
  {/if}
  </div>

  <KnownHostsManager bind:show={showKnownHosts} />
  <PeerTrustManager bind:show={showPeerTrust} />
  <KeyManager bind:show={showKeyManager} />
  <AuditLogView
    bind:show={showAuditLog}
    on:openSettings={(e) => openSettingsFromAudit(e.detail.tab)}
  />
  <SettingsDialog bind:show={showSettings} initialTab={settingsInitialTab} />
  <PluginsDialog bind:show={showPlugins} />
  <ScriptsDialog bind:show={showScripts} />
  <PluginCommandPalette bind:this={commandPalette} />
  <PeerTrustPrompt />
  <SessionPassphrasePrompt />

  {#if showHostKeyDialog}
    <HostKeyDialog
      show={showHostKeyDialog}
      host={hostKeyHost}
      keyType={hostKeyType}
      fingerprint={hostKeyFingerprint}
      keyBase64={hostKeyKeyBase64}
      isMismatch={hostKeyMismatch}
      on:accept={handleHostKeyAccept}
      on:cancel={handleHostKeyCancel}
    />
  {/if}
{/if}

<ErrorDialog />
<ConflictDialog />

<!--
  Outside the vault gate on purpose. A key is handed over on three occasions - creating a vault,
  finishing a recovery reset, and asking for a new one in settings - and two of those happen with
  the application already up. Mounting it here is the one place that covers all three.
-->
{#if $pendingRecoveryKey}
  <RecoveryKeyDialog />
{/if}

<style>
  .app-shell {
    display: flex;
    flex-direction: column;
    height: 100vh;
    overflow: hidden;
  }

  .app-layout {
    display: flex;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }

  .main-area {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-width: 0;
    overflow: hidden;
  }

  .plugin-status-bar {
    display: flex;
    gap: 12px;
    align-items: center;
    padding: 2px 10px;
    font-size: 10px;
    color: var(--text-secondary);
    background: var(--bg-secondary);
    border-top: 1px solid var(--border-color);
    min-height: 20px;
    flex-shrink: 0;
  }

  .status-item {
    white-space: nowrap;
  }

  .top-bar {
    display: flex;
    align-items: stretch;
    background: var(--bg-tertiary);
    border-bottom: 1px solid var(--border-color);
  }

  .top-bar :global(.tab-bar) {
    flex: 1;
    border-bottom: none;
  }

  .top-bar-spacer {
    flex: 1;
    min-width: 0;
    display: flex;
    justify-content: center;
  }

  .session-area {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    overflow: hidden;
    position: relative;
  }
</style>
