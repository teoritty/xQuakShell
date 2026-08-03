<script lang="ts">
  /** Selectable list of parsed hosts. Presentational: selection lives above. */
  import { createEventDispatcher } from 'svelte';
  import { KeyRound, Waypoints } from 'lucide-svelte';
  import type { SSHConfigHost } from '../../api/sshConfig';
  import { describeHost, masterState, countDuplicates } from './importSelection';

  export let hosts: SSHConfigHost[] = [];
  export let selected: Set<string> = new Set();

  const dispatch = createEventDispatcher<{
    toggle: string;
    selectAll: void;
    selectNone: void;
    selectNew: void;
  }>();

  $: master = masterState(hosts, selected);
  $: duplicates = countDuplicates(hosts);
  $: selectedCount = hosts.filter((h) => selected.has(h.alias)).length;
</script>

<div class="host-list">
  <div class="list-header">
    <label class="master">
      <input
        type="checkbox"
        checked={master === 'all'}
        indeterminate={master === 'some'}
        on:change={() => dispatch(master === 'all' ? 'selectNone' : 'selectAll')}
      />
      <span>{selectedCount} of {hosts.length} selected</span>
    </label>
    {#if duplicates > 0}
      <button class="link" on:click={() => dispatch('selectNew')}>
        Select new only ({hosts.length - duplicates})
      </button>
    {/if}
  </div>

  <div class="rows" role="group" aria-label="Hosts to import">
    {#each hosts as host (host.alias)}
      <label class="row" class:duplicate={host.duplicate}>
        <input
          type="checkbox"
          checked={selected.has(host.alias)}
          on:change={() => dispatch('toggle', host.alias)}
        />
        <span class="alias">{host.alias}</span>
        <span class="target">{describeHost(host)}</span>
        <span class="badges">
          {#if host.jumpAliases.length > 0}
            <span class="badge" title="Jump chain: {host.jumpAliases.join(' → ')}">
              <Waypoints size={10} />
              {host.jumpAliases.length}
            </span>
          {/if}
          {#if host.keyCount > 0}
            <span class="badge" title="{host.keyCount} referenced key file(s)">
              <KeyRound size={10} />
              {host.keyCount}
            </span>
          {/if}
          {#if host.duplicate}
            <span class="badge dup" title="A connection with this host, port and user already exists">
              Already in vault
            </span>
          {/if}
        </span>
      </label>
    {/each}
  </div>
</div>

<style>
  .host-list {
    display: flex;
    flex-direction: column;
    min-height: 0;
  }

  .list-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
    padding: 0 2px 6px;
    font-size: 11px;
    color: var(--text-secondary);
  }

  .master {
    display: flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
  }

  .link {
    background: none;
    border: none;
    padding: 0;
    color: var(--accent);
    font-size: 11px;
    cursor: pointer;
  }

  .link:hover {
    text-decoration: underline;
  }

  .rows {
    display: flex;
    flex-direction: column;
    max-height: 260px;
    overflow-y: auto;
    border: 1px solid var(--border-color);
    border-radius: 4px;
  }

  .row {
    display: grid;
    grid-template-columns: auto minmax(80px, 1fr) minmax(0, 1.6fr) auto;
    align-items: center;
    gap: 8px;
    padding: 5px 8px;
    font-size: 12px;
    cursor: pointer;
    border-bottom: 1px solid var(--border-color);
  }

  .row:last-child {
    border-bottom: none;
  }

  .row:hover {
    background: var(--bg-hover);
  }

  /* Muted, not hidden: the user can still opt a duplicate back in. */
  .row.duplicate .alias,
  .row.duplicate .target {
    opacity: 0.6;
  }

  .alias {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .target {
    color: var(--text-secondary);
    font-size: 11px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .badges {
    display: flex;
    align-items: center;
    gap: 4px;
  }

  .badge {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding: 1px 5px;
    font-size: 10px;
    color: var(--text-secondary);
    background: var(--bg-input);
    border: 1px solid var(--border-color);
    border-radius: 999px;
    white-space: nowrap;
  }

  .badge.dup {
    color: var(--text-primary);
  }
</style>
