<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { ConnectionProtocol } from '../../stores/api';
  import './connectionDetailsShared.css';

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

<label class="connection-detail-field">
  <span class="connection-detail-field-label">Name</span>
  <input type="text" bind:value={name} on:input={() => dispatch('dirty')} placeholder="My Server" />
</label>

<label class="connection-detail-field">
  <span class="connection-detail-field-label">Protocol</span>
  <select value={protocol} on:change={onProtocolChange}>
    {#each protocols as p}
      <option value={p.id}>{p.label}</option>
    {/each}
  </select>
</label>

<div class="connection-detail-field-row">
  <label class="connection-detail-field" style="flex:1">
    <span class="connection-detail-field-label">Host</span>
    <input type="text" bind:value={host} on:input={() => dispatch('dirty')} placeholder="192.168.1.1" />
  </label>
  <label class="connection-detail-field" style="width: calc(60px * var(--ui-scale))">
    <span class="connection-detail-field-label">Port</span>
    <input type="number" bind:value={port} on:input={() => dispatch('dirty')} min="1" max="65535" />
  </label>
</div>
