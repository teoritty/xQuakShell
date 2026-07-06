<script lang="ts">
  import { onMount } from 'svelte';
  import { Plus, Trash2 } from 'lucide-svelte';
  import type { ForwardRule } from '../../stores/appState';
  import type { PluginTunnelProviderContribution } from '../../stores/api';
  import {
    deleteForwardRule,
    getPluginContributions,
    listForwardRules,
    saveForwardRule,
    setForwardRuleEnabled,
  } from '../../stores/api';

  export let connectionId = '';

  let rules: ForwardRule[] = [];
  let tunnelProviders: PluginTunnelProviderContribution[] = [];
  let busy = false;
  let error = '';

  $: if (connectionId) {
    void loadRules();
  }

  onMount(async () => {
    const contrib = await getPluginContributions();
    tunnelProviders = contrib.tunnelProviders || [];
  });

  async function loadRules() {
    if (!connectionId) return;
    rules = await listForwardRules(connectionId);
  }

  function newRule(): ForwardRule {
    return {
      id: '',
      kind: 'local',
      bindAddress: '127.0.0.1',
      bindPort: 0,
      targetHost: '',
      targetPort: 0,
      enabled: true,
    };
  }

  function addRule() {
    rules = [...rules, newRule()];
  }

  async function persistRule(rule: ForwardRule) {
    if (!connectionId) return;
    busy = true;
    error = '';
    try {
      await saveForwardRule(connectionId, rule);
      await loadRules();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  async function removeRule(ruleId: string) {
    if (!connectionId || !ruleId) {
      rules = rules.filter((r) => r.id !== ruleId);
      return;
    }
    busy = true;
    error = '';
    try {
      await deleteForwardRule(connectionId, ruleId);
      await loadRules();
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  async function toggleEnabled(rule: ForwardRule) {
    if (!connectionId || !rule.id) return;
    const enabled = !rule.enabled;
    busy = true;
    error = '';
    try {
      await setForwardRuleEnabled(connectionId, rule.id, enabled);
      rule.enabled = enabled;
      rules = [...rules];
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    } finally {
      busy = false;
    }
  }

  function providersForPlugin(pluginId: string) {
    return tunnelProviders.filter((p) => p.pluginId === pluginId);
  }

  const pluginIds = () => [...new Set(tunnelProviders.map((p) => p.pluginId))];
</script>

<div class="forward-rules">
  <div class="section-header">
    <span class="label">Forward rules</span>
    <button class="ghost micro-btn" type="button" on:click={addRule} disabled={busy}>
      <Plus size={12} /> Add
    </button>
  </div>
  <p class="hint">Rules bind to 127.0.0.1 only. Dynamic rules are active while a session is open.</p>
  {#if error}
    <p class="error">{error}</p>
  {/if}
  {#each rules as rule, i (rule.id || `draft-${i}`)}
    <div class="rule-card">
      <div class="rule-row">
        <select bind:value={rule.kind}>
          <option value="local">Local (-L)</option>
          <option value="remote">Remote (-R)</option>
          <option value="dynamic">Dynamic (-D)</option>
        </select>
        <input type="text" bind:value={rule.bindAddress} placeholder="127.0.0.1" title="Bind address" />
        <input type="number" bind:value={rule.bindPort} placeholder="Port" min="1" max="65535" />
        <label class="enabled-toggle" title="Active while session is open">
          <input type="checkbox" checked={rule.enabled} on:change={() => toggleEnabled(rule)} />
          On
        </label>
        <button class="ghost danger micro-btn" type="button" on:click={() => removeRule(rule.id)} disabled={busy}>
          <Trash2 size={12} />
        </button>
      </div>
      {#if rule.kind === 'local' || rule.kind === 'remote'}
        <div class="rule-row">
          <input type="text" bind:value={rule.targetHost} placeholder="Target host" />
          <input type="number" bind:value={rule.targetPort} placeholder="Target port" min="1" max="65535" />
        </div>
      {:else}
        <div class="rule-row">
          <select bind:value={rule.pluginId}>
            <option value="">Plugin…</option>
            {#each pluginIds() as pid}
              <option value={pid}>{pid}</option>
            {/each}
          </select>
          <select bind:value={rule.providerId}>
            <option value="">Provider…</option>
            {#each providersForPlugin(rule.pluginId || '') as prov}
              <option value={prov.id}>{prov.label || prov.id}</option>
            {/each}
          </select>
        </div>
      {/if}
      {#if rule.id}
        <div class="rule-id">ID: {rule.id}</div>
      {/if}
      <button class="secondary tiny-btn" type="button" on:click={() => persistRule(rule)} disabled={busy}>
        Save rule
      </button>
    </div>
  {/each}
  {#if rules.length === 0}
    <div class="empty">No forward rules</div>
  {/if}
</div>

<style>
  .forward-rules { display: flex; flex-direction: column; gap: 6px; }
  .section-header { display: flex; align-items: center; justify-content: space-between; }
  .label { font-size: 11px; font-weight: 600; color: var(--text-secondary); }
  .hint { font-size: 10px; color: var(--text-secondary); margin: 0; }
  .error { font-size: 10px; color: var(--danger, #c00); margin: 0; }
  .rule-card {
    padding: 6px;
    background: var(--bg-tertiary);
    border-radius: 2px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .rule-row { display: flex; gap: 4px; align-items: center; flex-wrap: wrap; }
  .rule-row input, .rule-row select { font-size: 11px; flex: 1; min-width: 60px; }
  .rule-id { font-size: 9px; color: var(--text-secondary); font-family: monospace; }
  .enabled-toggle { font-size: 10px; display: flex; align-items: center; gap: 2px; white-space: nowrap; }
  .empty { font-size: 10px; color: var(--text-secondary); }
  .micro-btn { font-size: 11px; padding: 1px 6px; display: inline-flex; align-items: center; gap: 3px; }
  .tiny-btn { font-size: 10px; padding: 2px 6px; align-self: flex-start; }
</style>
