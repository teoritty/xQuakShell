<script lang="ts">
  import type { StoredKey } from '../../api/keys';

  export let keys: StoredKey[] = [];
  export let selectedId = '';
  export let filter = '';

  // The fingerprint is searchable alongside the label because it is what a server shows you, and
  // matching a key to a log line is the main reason anyone searches this list.
  $: visible = keys.filter((key) => {
    const needle = filter.trim().toLowerCase();
    if (!needle) return true;
    return (
      key.comment.toLowerCase().includes(needle) ||
      key.keyType.toLowerCase().includes(needle) ||
      (key.fingerprint || '').toLowerCase().includes(needle)
    );
  });

  function describe(key: StoredKey): string {
    return key.bits ? `${key.keyType} ${key.bits}` : key.keyType;
  }
</script>

<ul class="key-list" role="listbox" aria-label="SSH keys">
  {#each visible as key (key.id)}
    <li>
      <button
        class="key-row"
        class:selected={key.id === selectedId}
        role="option"
        aria-selected={key.id === selectedId}
        on:click={() => (selectedId = key.id)}
      >
        <span class="key-name">{key.comment || key.id}</span>
        <span class="key-meta">
          <span class="key-type">{describe(key)}</span>
          {#if key.policy === 'passphrase'}<span class="badge" title="Needs its passphrase to be used">passphrase</span>{/if}
          {#if key.nonExportable}<span class="badge">sealed</span>{/if}
          {#if key.migrationPending}<span class="badge warn" title="Upgrade not finished">unfinished</span>{/if}
        </span>
      </button>
    </li>
  {:else}
    <li class="empty">{filter ? 'No key matches that search.' : 'No keys stored yet.'}</li>
  {/each}
</ul>

<style>
  .key-list {
    list-style: none;
    margin: 0;
    padding: 0;
    overflow-y: auto;
    flex: 1;
  }

  .key-row {
    display: flex;
    flex-direction: column;
    gap: 3px;
    width: 100%;
    padding: 8px 10px;
    background: transparent;
    border: none;
    border-left: 2px solid transparent;
    color: var(--text-primary, #ddd);
    text-align: left;
    cursor: pointer;
    font: inherit;
  }

  .key-row:hover {
    background: var(--bg-hover, rgba(255, 255, 255, 0.05));
  }

  .key-row.selected {
    background: var(--bg-selected, rgba(255, 255, 255, 0.08));
    border-left-color: var(--accent, #4a9eff);
  }

  .key-name {
    font-size: 13px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .key-meta {
    display: flex;
    gap: 6px;
    align-items: center;
    font-size: 11px;
    color: var(--text-secondary, #888);
  }

  .badge {
    padding: 0 5px;
    border-radius: 3px;
    background: var(--bg-badge, rgba(255, 255, 255, 0.1));
    font-size: 10px;
    text-transform: uppercase;
    letter-spacing: 0.4px;
  }

  .badge.warn {
    background: var(--warn-bg, rgba(255, 176, 0, 0.18));
    color: var(--warn-fg, #ffb000);
  }

  .empty {
    padding: 14px 10px;
    color: var(--text-secondary, #888);
    font-size: 12px;
  }
</style>
