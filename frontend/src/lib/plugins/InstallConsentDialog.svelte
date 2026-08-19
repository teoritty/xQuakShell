<script lang="ts">
  // The one confirmation screen for every install, whatever the origin.
  //
  // The Install button stays disabled until every raised consent is ticked. That is a courtesy,
  // not the enforcement: the backend re-checks each grant in enforceInstallConsents and refuses an
  // install whose grants do not cover its warnings, so a caller that skipped this dialog entirely
  // still cannot install without consent.
  import { createEventDispatcher } from 'svelte';
  import { AlertTriangle, ShieldAlert } from 'lucide-svelte';
  import Modal from '../Modal.svelte';
  import {
    allConsentsGiven,
    NO_CONSENTS,
    type ConsentAnswers,
    type ConsentItem,
    type TrustWarning,
  } from './installConsent';

  export let show = false;
  export let title = 'Install plugin';
  /** Lines describing what is being installed: name, version, release tag. */
  export let summary: string[] = [];
  export let consents: ConsentItem[] = [];
  export let warnings: TrustWarning[] = [];
  /** Shown above everything when the install bypasses source checks. */
  export let originWarning = '';
  export let busy = false;
  /** Blocks the install outright, with a reason, when the preview says it cannot succeed. */
  export let blockedReason = '';

  const dispatch = createEventDispatcher();

  let answers: ConsentAnswers = { ...NO_CONSENTS };

  // Cleared on close, so the next open starts blank. Carrying a previous plugin's ticks forward
  // would pre-grant permissions the user answered for something else - and the two dialogs look
  // alike enough that nobody would notice.
  $: if (!show) answers = { ...NO_CONSENTS };

  $: ready = allConsentsGiven(consents, answers) && !blockedReason;

  function toggle(key: ConsentItem['key'], value: boolean) {
    answers = { ...answers, [key]: value };
  }
</script>

<Modal {title} {show} on:close={() => dispatch('cancel')}>
  <div class="consent-body">
    {#if originWarning}
      <div class="banner caution">
        <ShieldAlert size={14} />
        <span>{originWarning}</span>
      </div>
    {/if}

    {#if summary.length}
      <div class="summary">
        {#each summary as line}<div class="summary-line">{line}</div>{/each}
      </div>
    {/if}

    {#each warnings as warning}
      <div class="banner" class:critical={warning.severity === 'critical'} class:caution={warning.severity === 'caution'}>
        <AlertTriangle size={14} />
        <span>{warning.text}</span>
      </div>
    {/each}

    {#if consents.length}
      <div class="consent-list">
        <div class="consent-title">This plugin asks for:</div>
        {#each consents as item (item.key)}
          <label class="consent-row">
            <input
              type="checkbox"
              checked={answers[item.key]}
              on:change={(e) => toggle(item.key, e.currentTarget.checked)}
            />
            <span class="consent-text">
              <span class="consent-label">{item.label}</span>
              <span class="consent-detail">{item.detail}</span>
            </span>
          </label>
        {/each}
      </div>
    {/if}

    {#if blockedReason}
      <div class="banner critical">
        <AlertTriangle size={14} />
        <span>{blockedReason}</span>
      </div>
    {/if}
  </div>

  <div class="dialog-actions">
    <button class="secondary" disabled={busy} on:click={() => dispatch('cancel')}>Cancel</button>
    <button class="primary" disabled={!ready || busy} on:click={() => dispatch('confirm', { answers })}>
      {busy ? 'Installing…' : 'Install'}
    </button>
  </div>
</Modal>

<style>
  .consent-body {
    display: flex;
    flex-direction: column;
    gap: 10px;
    max-width: 60ch;
  }

  .summary {
    display: flex;
    flex-direction: column;
    gap: 2px;
    font-size: 12px;
  }

  .summary-line {
    overflow-wrap: anywhere;
  }

  .banner {
    display: flex;
    align-items: flex-start;
    gap: 7px;
    padding: 8px 10px;
    border-radius: 5px;
    font-size: 11.5px;
    line-height: 1.45;
  }

  .banner.caution {
    background: rgba(210, 153, 34, 0.12);
    color: var(--warning, #d29922);
  }

  .banner.critical {
    background: rgba(248, 81, 73, 0.12);
    color: var(--danger, #f85149);
  }

  .consent-list {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .consent-title {
    font-size: 12px;
    font-weight: 600;
  }

  .consent-row {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    cursor: pointer;
  }

  .consent-text {
    display: flex;
    flex-direction: column;
    gap: 1px;
  }

  .consent-label {
    font-size: 12px;
  }

  .consent-detail {
    font-size: 11px;
    color: var(--text-secondary);
  }
</style>
