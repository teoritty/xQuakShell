<script lang="ts">
  import { sessions, activeSessionId, connections } from '../stores/appState';
  import { closeSession } from '../stores/api';
  import { Loader2, CheckCircle2, XCircle, Circle, X, Monitor, MonitorPlay, Terminal, Cable, Globe } from 'lucide-svelte';

  function protocolIcon(connectionId: string) {
    const conn = $connections.find(c => c.id === connectionId);
    switch (conn?.protocol || 'ssh') {
      case 'rdp': return MonitorPlay;
      case 'telnet': return Terminal;
      case 'serial': return Cable;
      case 'http': return Globe;
      default: return Monitor;
    }
  }

  function protocolLabel(connectionId: string): string {
    const conn = $connections.find(c => c.id === connectionId);
    return (conn?.protocol || 'ssh').toUpperCase();
  }

  function activateTab(sessionId: string) {
    activeSessionId.set(sessionId);
  }

  async function closeTab(e: MouseEvent, sessionId: string) {
    e.stopPropagation();
    await closeSession(sessionId);
    if ($activeSessionId === sessionId) {
      const remaining = $sessions.filter(s => s.sessionId !== sessionId);
      activeSessionId.set(remaining.length > 0 ? remaining[remaining.length - 1].sessionId : '');
    }
  }
</script>

<div class="tab-bar">
  {#if $sessions.length === 0}
    <div class="no-tabs">No active sessions. Double-click a connection to start.</div>
  {/if}

  {#each $sessions as session (session.sessionId)}
    <div
      class="tab"
      class:active={$activeSessionId === session.sessionId}
      on:click={() => activateTab(session.sessionId)}
      on:keydown={(e) => e.key === 'Enter' && activateTab(session.sessionId)}
      role="tab"
      tabindex="0"
    >
      <span class="tab-state">
        {#if session.state === 'connecting'}
          <Loader2 size={11} />
        {:else if session.state === 'ready'}
          <CheckCircle2 size={11} style="color: #4caf50" />
        {:else if session.state === 'error'}
          <XCircle size={11} style="color: var(--danger)" />
        {:else}
          <Circle size={11} />
        {/if}
      </span>
      <span class="tab-protocol" title="Protocol">
        <svelte:component this={protocolIcon(session.connectionId)} size={11} />
        <span class="protocol-badge">{protocolLabel(session.connectionId)}</span>
      </span>
      <span class="tab-name">{session.connectionName || 'Session'}</span>
      <button class="tab-close" on:click={(e) => closeTab(e, session.sessionId)} title="Close session">
        <X size={11} />
      </button>
    </div>
  {/each}
</div>

<style>
  .tab-bar {
    display: flex;
    align-items: stretch;
    background: var(--bg-tertiary);
    border-bottom: 1px solid var(--border-color);
    overflow-x: auto;
    flex-shrink: 0;
    min-height: 34px;
  }

  .no-tabs {
    display: flex;
    align-items: center;
    padding: 0 12px;
    font-size: 12px;
    color: var(--text-secondary);
    font-style: italic;
    white-space: nowrap;
  }

  .tab {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 0 12px;
    min-width: 120px;
    max-width: 200px;
    cursor: pointer;
    user-select: none;
    background: var(--tab-inactive-bg);
    border-right: 1px solid var(--border-color);
    transition: background 0.1s;
    font-size: 12px;
  }

  .tab:hover { background: var(--bg-hover); }

  .tab.active {
    background: var(--tab-active-bg);
    border-bottom: 2px solid var(--accent);
    color: var(--text-bright);
  }

  .tab-state {
    display: inline-flex;
    align-items: center;
    flex-shrink: 0;
    color: var(--text-secondary);
  }

  .tab-protocol {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    flex-shrink: 0;
    color: var(--text-secondary);
    opacity: 0.8;
  }

  .tab-protocol .protocol-badge {
    font-size: 9px;
    font-weight: 600;
    letter-spacing: 0.3px;
  }

  .tab-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .tab-close {
    background: transparent;
    color: var(--text-secondary);
    padding: 0 3px;
    display: none;
    align-items: center;
    border: none;
    cursor: pointer;
    flex-shrink: 0;
  }

  .tab:hover .tab-close { display: inline-flex; }
  .tab-close:hover { color: var(--danger); }
</style>
