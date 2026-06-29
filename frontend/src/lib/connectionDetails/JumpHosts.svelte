<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { Plus, ChevronUp, ChevronDown } from 'lucide-svelte';
  import AuthEntryCard from '../AuthEntryCard.svelte';
  import type { JumpHop, SSHIdentityMeta } from '../../stores/appState';
  import type { AuthMethod } from './types';
  import { createDraftHopUiId } from './hopIds';
  import './connectionDetailsShared.css';

  export let jumpHops: JumpHop[] = [];
  export let identities: SSHIdentityMeta[] = [];

  const dispatch = createEventDispatcher<{
    dirty: void;
    hopschange: JumpHop[];
    keyimport: string;
    keyremove: { hopId: string; keyId: string };
    passwordchange: { hopId: string; value: string };
  }>();

  function addHop() {
    const id = createDraftHopUiId();
    dispatch('hopschange', [
      ...jumpHops,
      { id, host: '', port: 22, username: '', authMethod: 'key' as AuthMethod },
    ]);
    dispatch('dirty');
  }

  function removeHop(hopId: string) {
    dispatch('hopschange', jumpHops.filter((h) => h.id !== hopId));
    dispatch('dirty');
  }

  function updateHopField(hopId: string, field: keyof JumpHop, value: unknown) {
    dispatch(
      'hopschange',
      jumpHops.map((h) => (h.id === hopId ? { ...h, [field]: value } : h)),
    );
    dispatch('dirty');
  }

  function moveHop(hopId: string, direction: -1 | 1) {
    const idx = jumpHops.findIndex((h) => h.id === hopId);
    if (idx < 0) return;
    const newIdx = idx + direction;
    if (newIdx < 0 || newIdx >= jumpHops.length) return;
    const next = [...jumpHops];
    const [item] = next.splice(idx, 1);
    next.splice(newIdx, 0, item);
    dispatch('hopschange', next);
    dispatch('dirty');
  }
</script>

<div class="connection-detail-field">
  <div class="connection-detail-section-header">
    <span class="connection-detail-field-label">Jump Hosts (Bastion)</span>
    <button class="ghost connection-detail-micro-btn" on:click={addHop}><Plus size={12} /> Hop</button>
  </div>
  {#each jumpHops as hop, idx (hop.id)}
    <AuthEntryCard
      authMethod={hop.authMethod}
      keyAuth={hop.keyAuth}
      passAuth={hop.passAuth}
      {identities}
      on:authmethodchange={(e) => updateHopField(hop.id, 'authMethod', e.detail)}
      on:passwordchange={(e) => dispatch('passwordchange', { hopId: hop.id, value: e.detail })}
      on:keyimport={() => dispatch('keyimport', hop.id)}
      on:keyremove={(e) => dispatch('keyremove', { hopId: hop.id, keyId: e.detail })}
      on:remove={() => removeHop(hop.id)}
    >
      <svelte:fragment slot="primary">
        <div class="hop-fields">
          <div class="hop-field-row">
            <input
              type="text"
              value={hop.host}
              on:input={(e) => updateHopField(hop.id, 'host', e.currentTarget.value)}
              placeholder="host"
              class="hop-host"
            />
            <input
              type="number"
              value={hop.port}
              on:input={(e) => updateHopField(hop.id, 'port', parseInt(e.currentTarget.value) || 22)}
              min="1" max="65535"
              placeholder="port"
              class="hop-port"
            />
          </div>
          <input
            type="text"
            value={hop.username}
            on:input={(e) => updateHopField(hop.id, 'username', e.currentTarget.value)}
            placeholder="username"
            class="hop-username"
          />
        </div>
      </svelte:fragment>
      <svelte:fragment slot="meta">
        <div class="hop-reorder-stack">
          <button
            class="ghost hop-reorder"
            title="Move up"
            disabled={idx === 0}
            on:click={() => moveHop(hop.id, -1)}
          >
            <ChevronUp size={12} />
          </button>
          <span class="hop-badge" title="Hop order in chain">{idx + 1}</span>
          <button
            class="ghost hop-reorder"
            title="Move down"
            disabled={idx === jumpHops.length - 1}
            on:click={() => moveHop(hop.id, 1)}
          >
            <ChevronDown size={12} />
          </button>
        </div>
      </svelte:fragment>
    </AuthEntryCard>
  {/each}
  {#if jumpHops.length === 0}
    <div class="connection-detail-empty-state">No jump hosts configured</div>
  {/if}
</div>

<style>
  .hop-fields { display: flex; flex-direction: column; gap: 4px; flex: 1; min-width: 0; width: 100%; }
  .hop-field-row { display: flex; gap: 8px; width: 100%; min-width: 0; }
  .hop-host {
    width: auto;
    flex: 1 1 auto;
    font-size: 11px;
    min-width: 0;
  }
  .hop-username {
    width: 100%;
    font-size: 11px;
    min-width: 0;
  }
  .hop-port {
    width: calc(60px * var(--ui-scale));
    flex: 0 0 calc(60px * var(--ui-scale));
    font-size: 11px;
  }
  .hop-reorder-stack {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 1px;
    flex-shrink: 0;
  }
  .hop-badge {
    font-size: 10px;
    font-weight: 600;
    color: var(--text-secondary);
    background: var(--bg-secondary);
    padding: 0 5px;
    border-radius: 2px;
    white-space: nowrap;
    line-height: 1.4;
  }
  .hop-reorder {
    padding: 0 3px;
    line-height: 1;
    display: inline-flex;
    align-items: center;
  }
  .hop-reorder:disabled {
    opacity: 0.35;
    cursor: default;
  }
</style>
