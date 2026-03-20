<script lang="ts">
  import { onMount } from 'svelte';
  import Sidebar from './lib/Sidebar.svelte';
  import TabBar from './lib/TabBar.svelte';
  import SessionView from './lib/SessionView.svelte';
  import VaultUnlock from './lib/VaultUnlock.svelte';
  import KnownHostsManager from './lib/KnownHostsManager.svelte';
  import HostKeyDialog from './lib/HostKeyDialog.svelte';
  import AuditLogView from './lib/AuditLogView.svelte';
  import ErrorDialog from './lib/ErrorDialog.svelte';
  import SettingsDialog from './lib/SettingsDialog.svelte';
  import ScriptsDialog from './lib/ScriptsDialog.svelte';
  import { sessions, activeSessionId, vaultUnlocked, pendingHostKey, connections } from './stores/appState';
  import {
    subscribeToEvents, resolveHostKey, createNewConnectionInFolder,
    createSessionFromSelection, focusNextSessionTab, focusPrevSessionTab, closeActiveSession,
    getSettings, parseHotkeyEvent, DEFAULT_SESSION_HOTKEYS,
  } from './stores/api';
  import { Settings, FileText, Shield, MonitorDot, Terminal } from 'lucide-svelte';

  let showKnownHosts = false;
  let showAuditLog = false;
  let showSettings = false;
  let showScripts = false;
  let hotkeys = { ...DEFAULT_SESSION_HOTKEYS };

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
    await resolveHostKey(hostKeySessionId, action, hostKeyHost, hostKeyKeyBase64);
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

  function hotkeyLabel(input: string): string {
    return (input || '')
      .replace(/Meta/g, 'Win')
      .replace(/Control/g, 'Ctrl');
  }

  async function loadHotkeysFromSettings() {
    const s = await getSettings();
    if (!s) return;
    hotkeys = {
      create: s.sessionHotkeyCreate || DEFAULT_SESSION_HOTKEYS.create,
      next: s.sessionHotkeyNext || DEFAULT_SESSION_HOTKEYS.next,
      prev: s.sessionHotkeyPrev || DEFAULT_SESSION_HOTKEYS.prev,
      close: s.sessionHotkeyClose || DEFAULT_SESSION_HOTKEYS.close,
    };
  }

  onMount(() => {
    subscribeToEvents();
    if ($vaultUnlocked) loadHotkeysFromSettings();

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
        focusNextSessionTab();
        return;
      }
      if (combo === hotkeys.prev) {
        e.preventDefault();
        e.stopPropagation();
        focusPrevSessionTab();
        return;
      }
      if (combo === hotkeys.close) {
        e.preventDefault();
        e.stopPropagation();
        await closeActiveSession();
      }
    };
    window.addEventListener('keydown', hotkeyHandler, true);
    const settingsChanged = () => loadHotkeysFromSettings();
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
    };
  });

  $: if ($vaultUnlocked) {
    loadHotkeysFromSettings();
  }

</script>

{#if !$vaultUnlocked}
  <VaultUnlock />
{:else}
  <div class="app-layout">
    <Sidebar />
    <div class="main-area">
      <div class="top-bar">
        <TabBar />
        <div class="top-bar-actions">
          <button class="ghost top-btn" on:click={() => showScripts = true} title="Scripts">
            <Terminal size={14} />
          </button>
          <button class="ghost top-btn" on:click={() => showAuditLog = true} title="Audit Log">
            <FileText size={14} />
          </button>
          <button class="ghost top-btn" on:click={() => showKnownHosts = true} title="Known Hosts">
            <Shield size={14} />
          </button>
          <button class="ghost top-btn" on:click={() => showSettings = true} title="Settings">
            <Settings size={14} />
          </button>
        </div>
      </div>
      <div class="session-area">
        {#if $sessions.length === 0}
          <div class="welcome-screen">
            <h2>xQuakShell</h2>
            {#if $connections.length === 0}
              <p class="welcome-subtitle">No connections yet. Create your first one to get started.</p>
            {:else}
              <p class="welcome-subtitle">Start a session from the sidebar or use global hotkeys.</p>
            {/if}
            <div class="welcome-actions">
              <button class="primary welcome-btn" on:click={() => createNewConnectionInFolder('')}>
                <MonitorDot size={14} />
                New connection
              </button>
              <button class="ghost welcome-btn" on:click={() => showSettings = true}>
                <Settings size={14} />
                Open settings
              </button>
            </div>
            <div class="welcome-hints">
              <div class="hint"><span class="hint-key">{hotkeyLabel(hotkeys.create)}</span> Create new session</div>
              <div class="hint"><span class="hint-key">{hotkeyLabel(hotkeys.next)}</span> Next session tab</div>
              <div class="hint"><span class="hint-key">{hotkeyLabel(hotkeys.prev)}</span> Previous session tab</div>
              <div class="hint"><span class="hint-key">{hotkeyLabel(hotkeys.close)}</span> Close active session</div>
            </div>
          </div>
        {/if}
        {#each $sessions as session (session.sessionId)}
          <SessionView
            {session}
            active={session.sessionId === $activeSessionId}
          />
        {/each}
      </div>
    </div>
  </div>

  <KnownHostsManager bind:show={showKnownHosts} />
  <AuditLogView bind:show={showAuditLog} />
  <SettingsDialog bind:show={showSettings} />
  <ScriptsDialog bind:show={showScripts} />
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

<style>
  .app-layout {
    display: flex;
    flex: 1;
    height: 100vh;
    overflow: hidden;
  }

  .main-area {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-width: 0;
    overflow: hidden;
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

  .top-bar-actions {
    display: flex;
    align-items: center;
    padding: 0 4px;
    gap: 1px;
    flex-shrink: 0;
  }

  .top-btn {
    padding: 4px 6px;
    border-radius: 2px;
    display: inline-flex;
    align-items: center;
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

  .welcome-screen {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    flex: 1;
    gap: 12px;
    color: var(--text-secondary);
  }

  .welcome-screen h2 {
    font-size: 18px;
    font-weight: 600;
    color: var(--text-primary);
    margin: 0;
  }

  .welcome-screen p {
    font-size: 13px;
    margin: 0;
  }

  .welcome-subtitle {
    color: var(--text-secondary);
  }

  .welcome-actions {
    display: flex;
    gap: 8px;
    margin-top: 4px;
    flex-wrap: wrap;
    justify-content: center;
  }

  .welcome-hints {
    display: flex;
    flex-direction: column;
    gap: 6px;
    margin-top: 16px;
    padding: 16px;
    background: var(--bg-secondary);
    border-radius: 4px;
    border: 1px solid var(--border-color);
  }

  .hint {
    font-size: 12px;
    color: var(--text-secondary);
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .hint-key {
    font-family: var(--font-mono, monospace);
    background: var(--bg-tertiary);
    border: 1px solid var(--border-color);
    border-radius: 4px;
    padding: 2px 6px;
    color: var(--text-primary);
  }

  .welcome-btn {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    margin-top: 8px;
  }
</style>
