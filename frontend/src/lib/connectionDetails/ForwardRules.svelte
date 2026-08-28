<script lang="ts">
  import { t } from '../../i18n/messages';
  import { createEventDispatcher, onMount } from 'svelte';
  import { Plus, Trash2 } from 'lucide-svelte';
  import type { ForwardRule } from '../../stores/appState';
  import type { PluginTunnelProviderContribution } from '../../api/pluginRuntime';
  import { getPluginContributions } from '../../api/pluginRuntime';
  import { createDraftRuleUiId } from './forwardRuleIds';
  import './connectionDetailsShared.css';

  export let rules: ForwardRule[] = [];
  export let fieldErrors: Record<string, string> = {};

  const dispatch = createEventDispatcher<{
    dirty: void;
    ruleschange: ForwardRule[];
  }>();

  let tunnelProviders: PluginTunnelProviderContribution[] = [];

  onMount(async () => {
    const contrib = await getPluginContributions();
    tunnelProviders = contrib.tunnelProviders || [];
  });

  function newRule(): ForwardRule {
    return {
      id: createDraftRuleUiId(),
      kind: 'local',
      bindAddress: '127.0.0.1',
      bindPort: 0,
      targetHost: '',
      targetPort: 0,
      enabled: true,
    };
  }

  function addRule() {
    // Empty rule rows are local-only until required fields are filled; do not mark dirty
    // or autosave would persist without them and reconcile would erase the form.
    dispatch('ruleschange', [...rules, newRule()]);
  }

  function removeRule(ruleId: string) {
    dispatch('ruleschange', rules.filter((r) => r.id !== ruleId));
    dispatch('dirty');
  }

  function updateRuleField<K extends keyof ForwardRule>(ruleId: string, field: K, value: ForwardRule[K]) {
    dispatch(
      'ruleschange',
      rules.map((r) => (r.id === ruleId ? { ...r, [field]: value } : r)),
    );
    dispatch('dirty');
  }

  // Presentational only: it decides whether to show the acknowledgement, never whether the rule
  // is allowed. domain.ForwardRule.Validate is the authority and refuses the rule regardless of
  // what this returns.
  function isLoopbackBind(addr: string): boolean {
    const host = (addr || '').trim();
    if (!host) return true;
    return host === '127.0.0.1' || host === '::1' || host === 'localhost' || host.startsWith('127.');
  }

  function providersForPlugin(pluginId: string) {
    return tunnelProviders.filter((p) => p.pluginId === pluginId);
  }

  const pluginIds = () => [...new Set(tunnelProviders.map((p) => p.pluginId))];

  function ruleError(ruleId: string, field: string): string {
    return fieldErrors[`forwardRules.${ruleId}.${field}`] || fieldErrors[`forwardRules.${field}`] || '';
  }

  function onKindChange(ruleId: string, value: string) {
    updateRuleField(ruleId, 'kind', value as ForwardRule['kind']);
  }
</script>

<div class="connection-detail-field">
  <div class="connection-detail-section-header">
    <span class="connection-detail-field-label">{$t('connection.field.forwardRules')}</span>
    <button class="ghost connection-detail-micro-btn" type="button" on:click={addRule}>
      <Plus size={12} /> Add
    </button>
  </div>
  <p class="connection-detail-field-hint">{$t('security.connection.forward.hint')}</p>

  {#each rules as rule (rule.id)}
    <div class="forward-rule-card">
      <div class="forward-rule-fields">
        <div class="forward-rule-bind-row">
          <input
            type="text"
            value="127.0.0.1"
            readonly
            disabled
            title={$t('security.connection.forward.bindAddress')}
            class="bind-address"
          />
          <input
            type="number"
            value={rule.bindPort}
            on:input={(e) => updateRuleField(rule.id, 'bindPort', parseInt(e.currentTarget.value) || 0)}
            placeholder={$t('connection.forward.bindPort')}
            min="1"
            max="65535"
            class="bind-port"
          />
        </div>
        {#if rule.kind === 'local' || rule.kind === 'remote'}
          <div class="forward-rule-target-row">
            <input
              type="text"
              value={rule.targetHost || ''}
              on:input={(e) => updateRuleField(rule.id, 'targetHost', e.currentTarget.value)}
              placeholder={$t('connection.forward.targetHost')}
            />
            <input
              type="number"
              value={rule.targetPort || 0}
              on:input={(e) => updateRuleField(rule.id, 'targetPort', parseInt(e.currentTarget.value) || 0)}
              placeholder={$t('connection.forward.targetPort')}
              min="1"
              max="65535"
              class="target-port"
            />
          </div>
          {#if ruleError(rule.id, 'targetHost')}
            <p class="connection-detail-field-error">{ruleError(rule.id, 'targetHost')}</p>
          {/if}
          {#if rule.kind === 'remote' && !isLoopbackBind(rule.bindAddress)}
            <!-- Only shown for a rule that already binds beyond loopback. There is no bind-address
                 field in this form, so such a rule arrived by import or from an older vault; the
                 backend refuses it until it is acknowledged, and without this it could not be. -->
            <label class="gateway-ack">
              <input
                type="checkbox"
                checked={rule.allowRemoteGateway ?? false}
                on:change={(e) => updateRuleField(rule.id, 'allowRemoteGateway', e.currentTarget.checked)}
              />
              Publish on <code>{rule.bindAddress}</code> — reachable from the server's network, not just the server
            </label>
          {/if}
        {:else}
          <div class="forward-rule-target-row">
            <select
              value={rule.pluginId || ''}
              on:change={(e) => updateRuleField(rule.id, 'pluginId', e.currentTarget.value)}
            >
              <option value="">{$t('connection.forward.pluginPrompt')}</option>
              {#each pluginIds() as pid}
                <option value={pid}>{pid}</option>
              {/each}
            </select>
            <select
              value={rule.providerId || ''}
              on:change={(e) => updateRuleField(rule.id, 'providerId', e.currentTarget.value)}
              disabled={!rule.pluginId}
            >
              <option value="">{$t('connection.forward.providerPrompt')}</option>
              {#each providersForPlugin(rule.pluginId || '') as prov}
                <option value={prov.id}>{prov.label || prov.id}</option>
              {/each}
            </select>
          </div>
          <p class="consent-hint">{$t('security.connection.forward.consentHint')}</p>
        {/if}
        {#if ruleError(rule.id, 'bindPort')}
          <p class="connection-detail-field-error">{ruleError(rule.id, 'bindPort')}</p>
        {/if}
      </div>
      <div class="forward-rule-toolbar">
        <select
          value={rule.kind}
          on:change={(e) => onKindChange(rule.id, e.currentTarget.value)}
          class="kind-select"
        >
          <option value="local">{$t('connection.forward.local')}</option>
          <option value="remote">{$t('connection.forward.remote')}</option>
          <option value="dynamic">{$t('connection.forward.dynamic')}</option>
        </select>
        <div class="forward-rule-meta">
          <label class="enabled-toggle" title={$t('connection.forward.activeWhileOpen')}>
            <input
              type="checkbox"
              checked={rule.enabled}
              on:change={(e) => updateRuleField(rule.id, 'enabled', e.currentTarget.checked)}
            />
            On
          </label>
          <button
            class="ghost micro-btn danger toolbar-remove"
            type="button"
            on:click={() => removeRule(rule.id)}
            title={$t('common.remove')}
          >
            <Trash2 size={12} />
          </button>
        </div>
      </div>
    </div>
  {/each}

  {#if rules.length === 0}
    <div class="connection-detail-empty-state">No forward rules</div>
  {/if}
</div>

<style>
  .forward-rule-card {
    padding: 6px;
    background: var(--bg-tertiary);
    border-radius: 2px;
    margin-bottom: 4px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .forward-rule-fields {
    display: flex;
    flex-direction: column;
    gap: 4px;
    min-width: 0;
  }

  .forward-rule-bind-row,
  .forward-rule-target-row {
    display: flex;
    gap: 8px;
    min-width: 0;
  }

  .forward-rule-bind-row input,
  .forward-rule-target-row input,
  .forward-rule-target-row select {
    font-size: 11px;
    min-width: 0;
  }

  .bind-address {
    width: calc(88px * var(--ui-scale));
    flex: 0 0 calc(88px * var(--ui-scale));
  }

  .bind-port,
  .target-port {
    width: calc(72px * var(--ui-scale));
    flex: 0 0 calc(72px * var(--ui-scale));
  }

  .forward-rule-target-row input:first-child,
  .forward-rule-target-row select:first-child {
    flex: 1 1 auto;
  }

  .forward-rule-toolbar {
    display: flex;
    align-items: center;
    gap: 4px;
    min-width: 0;
  }

  .kind-select {
    width: 110px;
    font-size: 11px;
    flex-shrink: 0;
  }

  .forward-rule-meta {
    display: flex;
    align-items: center;
    gap: 4px;
    flex: 1;
    min-width: 0;
    justify-content: flex-end;
  }

  .enabled-toggle {
    font-size: 10px;
    display: flex;
    align-items: center;
    gap: 2px;
    white-space: nowrap;
  }

  .toolbar-remove {
    flex-shrink: 0;
  }

  .gateway-ack { display: flex; align-items: center; gap: 8px; font-size: 11px; line-height: 1.4; }
  .gateway-ack code { font-size: 11px; }
  .consent-hint {
    font-size: 9px;
    color: var(--text-secondary);
    margin: 0;
  }

  .micro-btn {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    font-size: 11px;
    padding: 1px 6px;
  }
</style>
