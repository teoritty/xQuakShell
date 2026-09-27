<script lang="ts">
  // What the session area shows before any tab is open: a way to start, and every shortcut. The
  // shortcuts are the user's own bindings, not the defaults, so the list stays true after a rebind.
  import { createEventDispatcher } from 'svelte';
  import { Settings, MonitorDot } from 'lucide-svelte';
  import { t } from '../i18n/messages';
  import { connections } from '../stores/appState';
  import { createNewConnectionInFolder } from '../actions/connectionActions';
  import { hotkeyLabel } from '../hotkeys/hotkeys';
  import type { AppHotkeys } from '../hotkeys/appHotkeys';

  export let hotkeys: AppHotkeys;

  const dispatch = createEventDispatcher<{ settings: void }>();

  $: hints = [
    { keys: hotkeys.create, label: 'settings.hotkeys.field.create' },
    { keys: hotkeys.next, label: 'settings.hotkeys.field.next' },
    { keys: hotkeys.prev, label: 'settings.hotkeys.field.prev' },
    { keys: hotkeys.close, label: 'settings.hotkeys.field.close' },
    { keys: hotkeys.localTerminal, label: 'settings.hotkeys.field.newLocalTerminal' },
    { keys: hotkeys.search, label: 'settings.hotkeys.field.terminalSearch' },
    { keys: hotkeys.multiInput, label: 'settings.hotkeys.field.multiInput' },
    { keys: hotkeys.multiInputStop, label: 'settings.hotkeys.field.multiInputStop' },
    { keys: 'Ctrl+Shift+P', label: 'welcome.commandPalette' },
  ];
</script>

<div class="welcome-screen">
  <h2>xQuakShell</h2>
  {#if $connections.length === 0}
    <p class="welcome-subtitle">{$t('welcome.noConnections')}</p>
  {:else}
    <p class="welcome-subtitle">{$t('welcome.startSession')}</p>
  {/if}
  <div class="welcome-actions">
    <button class="primary welcome-btn" on:click={() => createNewConnectionInFolder('')}>
      <MonitorDot size={14} />
      {$t('welcome.newConnection')}
    </button>
    <button class="ghost welcome-btn" on:click={() => dispatch('settings')}>
      <Settings size={14} />
      {$t('welcome.openSettings')}
    </button>
  </div>
  <div class="welcome-hints">
    {#each hints as hint (hint.label)}
      <div class="hint"><span class="hint-key">{hotkeyLabel(hint.keys)}</span> {$t(hint.label)}</div>
    {/each}
  </div>
</div>

<style>
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
