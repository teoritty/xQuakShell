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

  let levelFilter: 'all' | 'debug' | 'info' | 'warn' | 'error' = 'all';
  let search = '';

  const maxLines = 5000;

  // Rank levels so the filter can show "this level and above".
  const levelRank: Record<string, number> = { debug: 0, info: 1, warn: 2, warning: 2, error: 3 };

  function passesFilter(entry: LogEntry): boolean {
    if (levelFilter !== 'all') {
      const min = levelRank[levelFilter] ?? 0;
      const lvl = levelRank[(entry.level || 'info').toLowerCase()] ?? 1;
      if (lvl < min) return false;
    }
    const q = search.trim().toLowerCase();
    if (q) {
      const hay = `${entry.message} ${entry.source} ${fieldsText(entry.fields)}`.toLowerCase();
      if (!hay.includes(q)) return false;
    }
    return true;
  }

  $: visibleLines = lines.filter(passesFilter);

  function componentOf(entry: LogEntry): string {
    return entry.fields?.component ?? '';
  }

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
    // component is rendered as its own badge, so keep it out of the inline blob.
    const parts = Object.entries(fields)
      .filter(([k]) => k !== 'component')
      .map(([k, v]) => `${k}=${v}`);
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
      <select class="ghost" bind:value={levelFilter} title="Minimum level">
        <option value="all">All levels</option>
        <option value="debug">Debug+</option>
        <option value="info">Info+</option>
        <option value="warn">Warn+</option>
        <option value="error">Error</option>
      </select>
      <input class="search" type="text" placeholder="Search…" bind:value={search} />
      <button class="ghost" on:click={() => { paused = !paused; }}>{paused ? 'Resume' : 'Pause'}</button>
      <button class="ghost" on:click={clearLogs}>Clear</button>
    </div>
  </div>
  <div class="log-body" bind:this={logBody} on:scroll={onScroll}>
    {#each visibleLines as line, idx (idx)}
      <div class="log-line" class:drop-marker={line.source === 'loghub'}>
        <span class="log-time">{formatTime(line.time)}</span>
        <span class="log-level {levelClass(line.level)}">{(line.level || 'info').toUpperCase()}</span>
        <span class="log-source">[{line.source}]</span>
        {#if componentOf(line)}
          <span class="log-component">{componentOf(line)}</span>
        {/if}
        <span class="log-message">{line.message}{fieldsText(line.fields)}</span>
      </div>
    {/each}
    {#if visibleLines.length === 0}
      <div class="empty">
        {lines.length === 0 ? 'Waiting for log entries…' : 'No entries match the current filter.'}
      </div>
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

  .search {
    background: var(--bg-primary);
    color: var(--text-primary);
    border: 1px solid var(--border-color);
    border-radius: 3px;
    padding: 2px 6px;
    font-size: 11px;
    width: 140px;
  }

  .log-source {
    color: var(--text-secondary);
    flex-shrink: 0;
  }

  .log-component {
    flex-shrink: 0;
    color: var(--text-primary);
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 3px;
    padding: 0 4px;
    font-size: 10px;
  }

  .drop-marker {
    background: color-mix(in srgb, var(--danger) 12%, transparent);
    border-radius: 3px;
  }

  .log-message {
    color: var(--text-bright);
  }

  .empty {
    color: var(--text-secondary);
    padding: 12px 4px;
  }
</style>
