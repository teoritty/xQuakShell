<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { ConnectionProtocol } from '../../stores/api';

  export let name = '';
  export let protocol = 'ssh';
  export let host = '';
  export let port = 22;
  export let protocols: ConnectionProtocol[] = [];

  const dispatch = createEventDispatcher<{
    dirty: void;
    protocolchange: { protocol: string; defaultPort?: number };
  }>();

  function onProtocolChange(e: Event) {
    const next = (e.currentTarget as HTMLSelectElement).value;
    const def = protocols.find((p) => p.id === next);
    dispatch('protocolchange', { protocol: next, defaultPort: def?.defaultPort });
  }
</script>

<label class="field">
  <span class="field-label">Name</span>
  <input type="text" bind:value={name} on:input={() => dispatch('dirty')} placeholder="My Server" />
</label>

<label class="field">
  <span class="field-label">Protocol</span>
  <select value={protocol} on:change={onProtocolChange}>
    {#each protocols as p}
      <option value={p.id}>{p.label}</option>
    {/each}
  </select>
</label>

<div class="field-row">
  <label class="field" style="flex:1">
    <span class="field-label">Host</span>
    <input type="text" bind:value={host} on:input={() => dispatch('dirty')} placeholder="192.168.1.1" />
  </label>
  <label class="field" style="width: calc(60px * var(--ui-scale))">
    <span class="field-label">Port</span>
    <input type="number" bind:value={port} on:input={() => dispatch('dirty')} min="1" max="65535" />
  </label>
</div>

<style>
  .field { display: flex; flex-direction: column; gap: 2px; }
  .field-label { font-size: 11px; color: var(--text-secondary); font-weight: 500; }
  .field input, .field select { width: 100%; }
  .field-row { display: flex; gap: 8px; }
</style>
