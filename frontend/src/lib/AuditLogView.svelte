<script lang="ts">
  import Modal from './Modal.svelte';
  import { activeSessionId } from '../stores/appState';
  import { sendTerminalInput } from '../stores/api';
  import { Search, RotateCcw, Copy, Loader2, FileText, Trash2, Trash } from 'lucide-svelte';

  export let show = false;

  interface AuditEntry {
    id: number;
    timestamp: string;
    sessionId: string;
    connectionId: string;
    username: string;
    input: string;
    redacted: boolean;
  }

  let query = '';
  let results: AuditEntry[] = [];
  let loading = false;
  let sessionFilter = '';
  let connectionFilter = '';
  let copiedId: number | null = null;
  let clearConfirmShow = false;

  function getApp() { return (window as any).go?.main?.App; }

  $: if (show) search();

  async function search() {
    const app = getApp();
    if (!app) return;
    loading = true;
    try {
      results = (await app.SearchAuditLog(query, sessionFilter, connectionFilter, 200, 0)) || [];
    } catch {
      results = [];
    }
    loading = false;
  }

  function formatTs(ts: string): string {
    try {
      return new Date(ts).toLocaleString();
    } catch { return ts; }
  }

  function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Enter') search();
  }

  async function rerunCommand(entry: AuditEntry) {
    const sid = $activeSessionId;
    if (!sid) return;
    await sendTerminalInput(sid, entry.input + '\n');
    show = false;
  }

  async function deleteEntry(entry: AuditEntry) {
    const app = getApp();
    if (!app?.DeleteAuditEntry) return;
    try {
      await app.DeleteAuditEntry(entry.id);
      await search();
    } catch {}
  }

  async function clearAll() {
    const app = getApp();
    if (!app?.ClearAuditLog) return;
    try {
      await app.ClearAuditLog();
      clearConfirmShow = false;
      await search();
    } catch {}
  }

  async function copyCommand(entry: AuditEntry) {
    try {
      await navigator.clipboard.writeText(entry.input);
      copiedId = entry.id;
      setTimeout(() => { if (copiedId === entry.id) copiedId = null; }, 1500);
    } catch {}
  }
</script>

{#if show}
  <Modal title="Audit Log" show={true} on:close={() => show = false}>
    <div class="audit-log">
      <div class="search-bar">
        <div class="search-input-wrap">
          <Search size={13} />
          <input
            type="text"
            bind:value={query}
            placeholder="Search commands..."
            on:keydown={handleKeydown}
            class="search-input"
          />
        </div>
        <button class="primary search-btn" on:click={search} disabled={loading}>
          {#if loading}<Loader2 size={13} />{:else}<Search size={13} />{/if}
          {loading ? 'Searching...' : 'Search'}
        </button>
        <button class="danger search-btn" on:click={() => clearConfirmShow = true} disabled={loading || results.length === 0} title="Clear all">
          <Trash size={13} />
          Clear all
        </button>
      </div>

      {#if clearConfirmShow}
        <div class="confirm-overlay">
          <div class="confirm-box">
            <p>Clear all audit log entries? This cannot be undone.</p>
            <div class="confirm-actions">
              <button class="primary" on:click={clearAll}>Clear all</button>
              <button class="secondary" on:click={() => clearConfirmShow = false}>Cancel</button>
            </div>
          </div>
        </div>
      {/if}

      <div class="filters">
        <input type="text" bind:value={sessionFilter} placeholder="Session ID filter" class="filter-input" />
        <input type="text" bind:value={connectionFilter} placeholder="Connection ID filter" class="filter-input" />
      </div>

      <div class="results">
        {#if results.length === 0 && !loading}
          <div class="empty">
            <FileText size={24} />
            <span>No audit log entries found</span>
          </div>
        {/if}
        {#each results as entry (entry.id)}
          <div class="entry" class:redacted={entry.redacted}>
            <div class="entry-header">
              <span class="entry-time">{formatTs(entry.timestamp)}</span>
              <span class="entry-user">{entry.username || 'unknown'}</span>
              {#if entry.redacted}
                <span class="redacted-badge">REDACTED</span>
              {/if}
              <div class="entry-actions">
                <button
                  class="entry-btn"
                  on:click={() => copyCommand(entry)}
                  title="Copy command"
                >
                  <Copy size={11} />
                  {#if copiedId === entry.id}<span class="copied-label">Copied</span>{/if}
                </button>
                <button
                  class="entry-btn danger"
                  on:click={() => deleteEntry(entry)}
                  title="Delete"
                >
                  <Trash2 size={11} />
                </button>
                {#if $activeSessionId && !entry.redacted}
                  <button
                    class="entry-btn rerun"
                    on:click={() => rerunCommand(entry)}
                    title="Re-run in active session"
                  >
                    <RotateCcw size={11} />
                    Re-run
                  </button>
                {/if}
              </div>
            </div>
            <div class="entry-input">{entry.input}</div>
          </div>
        {/each}
      </div>
    </div>
  </Modal>
{/if}

<style>
  .audit-log {
    position: relative;
    min-width: 500px;
    max-width: 700px;
    max-height: 70vh;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .search-bar {
    display: flex;
    gap: 6px;
    align-items: center;
  }

  .search-input-wrap {
    display: flex;
    align-items: center;
    gap: 6px;
    flex: 1;
    padding: 0 8px;
    background: var(--bg-input, var(--bg-primary));
    border: 1px solid var(--border-color);
    border-radius: 3px;
    color: var(--text-secondary);
  }
  .search-input-wrap:focus-within {
    border-color: var(--border-focus, var(--accent));
  }

  .search-input {
    flex: 1;
    font-size: 13px;
    border: none;
    background: transparent;
    outline: none;
    color: var(--text-primary);
    padding: 5px 0;
  }

  .search-btn {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    font-size: 12px;
    padding: 5px 12px;
    white-space: nowrap;
  }

  .filters {
    display: flex;
    gap: 6px;
  }

  .filter-input {
    flex: 1;
    font-size: 11px;
  }

  .results {
    overflow-y: auto;
    flex: 1;
    min-height: 0;
    max-height: 50vh;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .empty {
    color: var(--text-secondary);
    font-size: 12px;
    text-align: center;
    padding: 30px 20px;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
  }

  .entry {
    padding: 6px 8px;
    background: var(--bg-tertiary);
    border-radius: 4px;
    font-size: 12px;
  }

  .entry.redacted {
    opacity: 0.6;
  }

  .entry-header {
    display: flex;
    gap: 8px;
    align-items: center;
    margin-bottom: 2px;
  }

  .entry-time {
    color: var(--text-secondary);
    font-size: 10px;
  }

  .entry-user {
    color: var(--accent);
    font-size: 10px;
  }

  .redacted-badge {
    font-size: 9px;
    padding: 0 4px;
    background: var(--warning);
    color: #000;
    border-radius: 3px;
  }

  .entry-actions {
    margin-left: auto;
    display: flex;
    gap: 4px;
  }

  .entry-btn {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    font-size: 10px;
    padding: 1px 6px;
    background: transparent;
    border: 1px solid var(--border-color);
    color: var(--text-secondary);
    border-radius: 2px;
    cursor: pointer;
    white-space: nowrap;
  }
  .entry-btn:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }
  .entry-btn.rerun {
    color: var(--accent);
    border-color: var(--accent);
  }
  .entry-btn.rerun:hover {
    background: rgba(75, 139, 191, 0.1);
  }

  .entry-btn.danger:hover {
    color: var(--danger);
    border-color: var(--danger);
    background: rgba(211, 47, 47, 0.1);
  }

  .search-btn.danger {
    color: var(--danger);
    border-color: var(--danger);
  }
  .search-btn.danger:hover {
    background: rgba(211, 47, 47, 0.1);
  }

  .confirm-overlay {
    position: absolute;
    inset: 0;
    background: rgba(0, 0, 0, 0.5);
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 10;
    border-radius: 6px;
  }

  .confirm-box {
    background: var(--bg-secondary);
    padding: 16px;
    border-radius: 6px;
    border: 1px solid var(--border-color);
    max-width: 320px;
  }

  .confirm-box p {
    margin: 0 0 12px;
    font-size: 13px;
    color: var(--text-primary);
  }

  .confirm-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }

  .copied-label {
    color: var(--success, #4caf50);
    font-size: 9px;
  }

  .entry-input {
    font-family: var(--font-mono);
    white-space: pre-wrap;
    word-break: break-all;
    color: var(--text-primary);
  }
</style>
