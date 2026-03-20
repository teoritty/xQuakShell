<script lang="ts">
  import { onMount } from 'svelte';
  import Modal from './Modal.svelte';

  export let show = false;

  interface KnownHostEntry {
    host: string;
    keyType: string;
    fingerprint: string;
  }

  let entries: KnownHostEntry[] = [];
  let loading = false;
  let error = '';

  function getApp() {
    return (window as any).go?.main?.App;
  }

  async function loadEntries() {
    const app = getApp();
    if (!app) return;
    loading = true;
    error = '';
    try {
      entries = (await app.GetKnownHosts()) || [];
    } catch (e: any) {
      error = e?.message || String(e);
    } finally {
      loading = false;
    }
  }

  async function removeEntry(host: string) {
    if (!confirm(`Remove known host entry for "${host}"?\nThe next connection to this host will ask to verify its key again.`)) return;
    const app = getApp();
    if (!app) return;
    try {
      await app.RemoveKnownHost(host);
      await loadEntries();
    } catch (e: any) {
      error = e?.message || String(e);
    }
  }

  $: if (show) {
    loadEntries();
  }
</script>

<Modal title="Known Hosts" {show} on:close={() => show = false}>
  {#if error}
    <div class="kh-error">{error}</div>
  {/if}

  {#if loading}
    <div class="kh-loading">Loading...</div>
  {:else if entries.length === 0}
    <div class="kh-empty">No known hosts stored yet.</div>
  {:else}
    <div class="kh-list">
      {#each entries as entry (entry.host + entry.keyType)}
        <div class="kh-item">
          <div class="kh-info">
            <div class="kh-host">{entry.host}</div>
            <div class="kh-meta">
              <span class="kh-type">{entry.keyType}</span>
              <span class="kh-fp" title={entry.fingerprint}>{entry.fingerprint}</span>
            </div>
          </div>
          <button class="danger kh-remove" on:click={() => removeEntry(entry.host)} title="Remove host key">✕</button>
        </div>
      {/each}
    </div>
  {/if}

  <div class="kh-footer">
    <button class="secondary" on:click={() => loadEntries()}>Refresh</button>
  </div>
</Modal>

<style>
  .kh-error {
    padding: 8px 12px;
    background: rgba(211, 47, 47, 0.15);
    border: 1px solid var(--danger);
    border-radius: 4px;
    color: var(--danger);
    font-size: 12px;
    margin-bottom: 12px;
  }

  .kh-loading, .kh-empty {
    text-align: center;
    padding: 20px;
    color: var(--text-secondary);
    font-size: 13px;
  }

  .kh-list {
    display: flex;
    flex-direction: column;
    gap: 4px;
    max-height: 300px;
    overflow-y: auto;
  }

  .kh-item {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 10px;
    background: var(--bg-tertiary);
    border-radius: 4px;
  }

  .kh-info {
    flex: 1;
    min-width: 0;
  }

  .kh-host {
    font-size: 13px;
    font-weight: 500;
    color: var(--text-bright);
  }

  .kh-meta {
    display: flex;
    gap: 8px;
    margin-top: 2px;
  }

  .kh-type {
    font-size: 11px;
    color: var(--accent);
    font-family: var(--font-mono);
  }

  .kh-fp {
    font-size: 10px;
    color: var(--text-secondary);
    font-family: var(--font-mono);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .kh-remove {
    padding: 4px 8px;
    font-size: 11px;
    flex-shrink: 0;
  }

  .kh-footer {
    display: flex;
    justify-content: flex-end;
    margin-top: 12px;
  }

  .kh-footer button {
    font-size: 12px;
    padding: 5px 12px;
  }
</style>
