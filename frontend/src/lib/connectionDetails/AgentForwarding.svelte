<script lang="ts">
  // The connection's half of agent forwarding (ADR-022). Only keys whose own settings allow
  // forwarding are offered; the other half of the consent lives in the Key Manager.
  import { createEventDispatcher } from 'svelte';
  import { t } from '../../i18n/messages';
  import type { AgentForwarding, SSHIdentityMeta } from '../../stores/appState';
  import {
    forwardableKeys,
    listedWithoutConsent,
    setConfirmPolicy,
    setForwardingEnabled,
    setWindowMinutes,
    toggleForwardedKey,
    MAX_WINDOW_MINUTES,
  } from './agentForwardingDraft';
  import './connectionDetailsShared.css';

  export let agentForwarding: AgentForwarding | null = null;
  export let identities: SSHIdentityMeta[] = [];

  const dispatch = createEventDispatcher<{ change: AgentForwarding; dirty: void }>();

  $: enabled = agentForwarding?.enabled === true;
  $: offered = forwardableKeys(identities);
  $: withdrawn = listedWithoutConsent(agentForwarding, identities);

  function emit(next: AgentForwarding) {
    dispatch('change', next);
    dispatch('dirty');
  }
</script>

<div class="connection-detail-field">
  <label class="connection-detail-checkbox">
    <input type="checkbox" checked={enabled} on:change={(e) => emit(setForwardingEnabled(agentForwarding, e.currentTarget.checked))} />
    <span class="connection-detail-field-label">{$t('security.agent.editor.enable')}</span>
  </label>

  {#if enabled}
    <div class="connection-detail-field-hint af-risk">{$t('security.agent.editor.risk')}</div>

    {#if offered.length === 0}
      <div class="connection-detail-empty-state">{$t('security.agent.editor.noKeys')}</div>
    {:else}
      {#each offered as key (key.id)}
        <label class="connection-detail-checkbox">
          <input
            type="checkbox"
            checked={agentForwarding?.identityIds.includes(key.id) ?? false}
            on:change={() => emit(toggleForwardedKey(agentForwarding, key.id))}
          />
          <span>{key.comment || key.id} <code>{key.fingerprint ?? ''}</code></span>
        </label>
      {/each}
    {/if}

    {#each withdrawn as key (key.id)}
      <div class="connection-detail-field-error">{$t('security.agent.editor.withdrawn', { key: key.comment || key.id })}</div>
    {/each}

    <label class="connection-detail-field-label" for="af-confirm">{$t('security.agent.editor.confirm')}</label>
    <select
      id="af-confirm"
      value={agentForwarding?.confirm ?? 'each'}
      on:change={(e) => emit(setConfirmPolicy(agentForwarding, e.currentTarget.value === 'session-window' ? 'session-window' : 'each'))}
    >
      <option value="each">{$t('security.agent.editor.confirmEach')}</option>
      <option value="session-window">{$t('security.agent.editor.confirmWindow')}</option>
    </select>

    {#if agentForwarding?.confirm === 'session-window'}
      <label class="connection-detail-field-label" for="af-window">{$t('security.agent.editor.windowMinutes')}</label>
      <input
        id="af-window"
        type="number"
        min="1"
        max={MAX_WINDOW_MINUTES}
        value={agentForwarding.windowMinutes}
        on:change={(e) => emit(setWindowMinutes(agentForwarding, Number(e.currentTarget.value)))}
      />
    {/if}
  {/if}
</div>

<style>
  .af-risk { line-height: 1.4; }
  code { font-size: 10px; color: var(--text-secondary); }
</style>
