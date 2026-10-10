<script lang="ts">
  // Asks whether a server may have a signature made with a vault key (ADR-022). Presentational:
  // AgentConfirmPrompt owns the queue and the answer.
  //
  // Every string is in the security namespace, which a language pack cannot reword: this dialog is
  // the only thing between a server and a signature, and its wording is part of that boundary.
  //
  // Deny is the default in every way the dialog has one - the focused button, Escape, closing it -
  // because the request comes from the server, not from the user, and an accidental keypress must
  // not be the thing that approves it.
  import { createEventDispatcher, onMount } from 'svelte';
  import Modal from './Modal.svelte';
  import { t } from '../i18n/messages';
  import type { AgentConfirmEvent } from '../stores/agentConfirmState';
  import type { AgentDecision } from '../api/agentForwarding';

  export let request: AgentConfirmEvent;

  const dispatch = createEventDispatcher<{ decide: AgentDecision }>();
  let denyButton: HTMLButtonElement;

  onMount(() => denyButton?.focus());

  $: destinationName = request.knownAs.length > 0 ? request.knownAs.join(', ') : '';
</script>

<Modal title={$t('security.agent.title')} show={true} on:close={() => dispatch('decide', 'deny')}>
  <p class="ac-body">
    {$t('security.agent.body', { connection: request.connectionName, host: request.host, user: request.remoteUser })}
  </p>

  <dl class="ac-facts">
    <dt>{$t('security.agent.key')}</dt>
    <dd>{request.keyLabel} <code>{request.keyFingerprint}</code></dd>
    <dt>{$t('security.agent.destination')}</dt>
    <dd>
      {#if request.verified && destinationName}
        {destinationName} <code>{request.destination}</code>
        <span class="ac-ok">{$t('security.agent.verified')}</span>
      {:else if request.verified}
        {$t('security.agent.unknownHost')} <code>{request.destination}</code>
      {:else}
        <span class="ac-warn" role="alert">{$t('security.agent.unverified')}</span>
      {/if}
    </dd>
  </dl>

  {#if request.forwardsFurther}
    <p class="ac-warn">{$t('security.agent.forwardsFurther')}</p>
  {/if}
  <p class="ac-risk">{$t('security.agent.risk', { host: request.host })}</p>

  <div class="ac-actions">
    <button type="button" class="secondary" bind:this={denyButton} on:click={() => dispatch('decide', 'deny')}>
      {$t('security.agent.deny')}
    </button>
    <button type="button" class="primary" on:click={() => dispatch('decide', 'once')}>
      {$t('security.agent.allowOnce')}
    </button>
    {#if request.windowMinutes > 0}
      <button type="button" class="primary" on:click={() => dispatch('decide', 'window')}>
        {$t('security.agent.allowWindow', { minutes: request.windowMinutes })}
      </button>
    {/if}
  </div>
</Modal>

<style>
  .ac-body {
    margin: 0 0 10px;
    font-size: 13px;
    line-height: 1.5;
    color: var(--text-primary);
  }
  .ac-facts {
    display: grid;
    grid-template-columns: max-content 1fr;
    gap: 4px 10px;
    margin: 0 0 10px;
    font-size: 12px;
  }
  .ac-facts dt { color: var(--text-secondary); }
  .ac-facts dd { margin: 0; overflow-wrap: anywhere; }
  code { font-size: 11px; color: var(--text-secondary); }
  .ac-ok { color: #4caf50; font-size: 11px; }
  .ac-warn { color: var(--danger); font-size: 12px; margin: 0 0 8px; }
  .ac-risk { color: var(--text-secondary); font-size: 11px; line-height: 1.4; margin: 0; }
  .ac-actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
    margin-top: 14px;
  }
</style>
