<script lang="ts">
  import { onDestroy, onMount, tick } from 'svelte';

  interface LogEntry {
    time: string;
    level: string;
    source: string;
    message: string;
    fields?: Record<string, string>;
  }

  let lines: LogEntry[] = [];
  let paused = false;
  let logBody: HTMLDivElement;
  let shouldStick = true;

  const maxLines = 5000;

  function formatTime(raw: string): string {
    const d = new Date(raw);
    if (Number.isNaN(d.getTime())) return raw;
    return d.toLocaleTimeString(undefined, { hour12: false, fractionalSecondDigits: 3 });
  }

  function levelClass(level: string): string {
    switch ((level || '').toLowerCase()) {
      case 'debug':
        return 'level-debug';
      case 'warn':
      case 'warning':
        return 'level-warn';
      case 'error':
        return 'level-error';
      default:
        return 'level-info';
    }
  }

  function fieldsText(fields?: Record<string, string>): string {
    if (!fields) return '';
    const parts = Object.entries(fields).map(([k, v]) => `${k}=${v}`);
    return parts.length ? ` {${parts.join(' ')}}` : '';
  }

  function appendEntry(entry: LogEntry) {
    lines = [...lines, entry].slice(-maxLines);
    if (!paused && shouldStick) {
      tick().then(() => {
        if (logBody) logBody.scrollTop = logBody.scrollHeight;
      });
    }
  }

  function onScroll() {
    if (!logBody) return;
    const nearBottom = logBody.scrollHeight - logBody.scrollTop - logBody.clientHeight < 40;
    shouldStick = nearBottom;
  }

  function clearLogs() {
    lines = [];
  }

  onMount(() => {
    const rt = (window as any).runtime;
    if (!rt?.EventsOn) return;
    return rt.EventsOn('DebugLogLine', (entry: LogEntry) => {
      appendEntry(entry);
    });
  });

  onDestroy(() => {
    const rt = (window as any).runtime;
    if (rt?.EventsOff) rt.EventsOff('DebugLogLine');
  });
</script>

<div class="log-viewer">
  <div class="toolbar">
    <span class="title">Debug Log</span>
    <div class="toolbar-actions">
      <button class="ghost" on:click={() => { paused = !paused; }}>{paused ? 'Resume' : 'Pause'}</button>
      <button class="ghost" on:click={clearLogs}>Clear</button>
    </div>
  </div>
  <div class="log-body" bind:this={logBody} on:scroll={onScroll}>
    {#each lines as line, idx (idx)}
      <div class="log-line">
        <span class="log-time">{formatTime(line.time)}</span>
        <span class="log-level {levelClass(line.level)}">{(line.level || 'info').toUpperCase()}</span>
        <span class="log-source">[{line.source}]</span>
        <span class="log-message">{line.message}{fieldsText(line.fields)}</span>
      </div>
    {/each}
    {#if lines.length === 0}
      <div class="empty">Waiting for log entries…</div>
    {/if}
  </div>
</div>

<style>
  :global(html, body, #app) {
    height: 100%;
    overflow: hidden;
  }

  .log-viewer {
    display: flex;
    flex-direction: column;
    height: 100vh;
    background: var(--bg-primary);
    color: var(--text-primary);
  }

  .toolbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 6px 10px;
    border-bottom: 1px solid var(--border-color);
    background: var(--bg-secondary);
    flex-shrink: 0;
  }

  .title {
    font-size: 11px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.5px;
    color: var(--text-secondary);
  }

  .toolbar-actions {
    display: flex;
    gap: 4px;
  }

  .log-body {
    flex: 1;
    overflow: auto;
    padding: 6px 8px;
    font-family: var(--font-mono);
    font-size: 11px;
    line-height: 1.45;
  }

  .log-line {
    display: flex;
    gap: 8px;
    align-items: baseline;
    padding: 1px 0;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .log-time {
    color: var(--text-secondary);
    flex-shrink: 0;
  }

  .log-level {
    flex-shrink: 0;
    font-weight: 600;
    min-width: 44px;
  }

  .level-debug { color: #8a8a8a; }
  .level-info { color: #7eb8da; }
  .level-warn { color: #c49040; }
  .level-error { color: var(--danger); }

  .log-source {
    color: var(--text-secondary);
    flex-shrink: 0;
  }

  .log-message {
    color: var(--text-bright);
  }

  .empty {
    color: var(--text-secondary);
    padding: 12px 4px;
  }
</style>
