<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { AlertTriangle, Check, Copy, Download } from 'lucide-svelte';
  import { pendingRecoveryKey } from '../../stores/appState';
  import { acknowledgeRecoveryKey } from '../../actions/vaultActions';
  import { saveRecoveryKeyFileRpc } from '../../api/vault';
  import {
    RECOVERY_ACKNOWLEDGE_SECONDS,
    acknowledgeLabel,
    isAcknowledgeEnabled,
    tickCountdown,
  } from './recoveryCountdown';
  import { t } from '../../i18n/messages';

  // There is deliberately no close event, no backdrop handler and no Escape key. This dialog is the
  // only time the key is ever shown; dismissing it any other way would leave the user believing
  // they still had a chance to write it down.
  let secondsLeft = RECOVERY_ACKNOWLEDGE_SECONDS;
  let copied = false;
  let saved = false;
  let error = '';
  let timer: ReturnType<typeof setInterval> | undefined;
  let copiedTimer: ReturnType<typeof setTimeout> | undefined;

  $: key = $pendingRecoveryKey ?? '';
  $: canAcknowledge = isAcknowledgeEnabled(secondsLeft);

  onMount(() => {
    timer = setInterval(() => {
      secondsLeft = tickCountdown(secondsLeft);
      if (secondsLeft === 0 && timer) {
        clearInterval(timer);
        timer = undefined;
      }
    }, 1000);
  });

  onDestroy(() => {
    if (timer) clearInterval(timer);
    if (copiedTimer) clearTimeout(copiedTimer);
  });

  async function copy() {
    error = '';
    try {
      await navigator.clipboard.writeText(key);
      copied = true;
      if (copiedTimer) clearTimeout(copiedTimer);
      copiedTimer = setTimeout(() => {
        copied = false;
      }, 2000);
    } catch {
      error = $t('vault.recovery.copyFailed');
    }
  }

  async function download() {
    error = '';
    try {
      // The key itself never crosses back over the bridge: the backend writes the copy it is
      // already holding, and answers false when the user cancelled the picker.
      saved = await saveRecoveryKeyFileRpc();
    } catch (e: any) {
      error = e?.message || $t('vault.recovery.saveFailed');
    }
  }

  async function done() {
    if (!canAcknowledge) return;
    await acknowledgeRecoveryKey();
  }
</script>

<div class="backdrop">
  <div class="dialog" role="dialog" aria-modal="true" aria-label={$t('vault.recovery.title')}>
    <h2>{$t('vault.recovery.title')}</h2>
    <p class="subtitle">{$t('vault.recovery.subtitle')}</p>

    <p class="key" aria-label={$t('vault.recovery.keyLabel')}>{key}</p>

    <div class="warning" role="note">
      <AlertTriangle size={16} strokeWidth={2} />
      <span>{$t('vault.recovery.warning')}</span>
    </div>

    <p class="error" role="alert">{error}</p>

    <div class="actions">
      <button type="button" class="secondary" on:click={copy}>
        {#if copied}<Check size={14} />{:else}<Copy size={14} />{/if}
        {copied ? $t('vault.recovery.copied') : $t('vault.recovery.copy')}
      </button>
      <button type="button" class="secondary" on:click={download}>
        {#if saved}<Check size={14} />{:else}<Download size={14} />{/if}
        {saved ? $t('vault.recovery.downloaded') : $t('vault.recovery.download')}
      </button>
      <button type="button" class="primary done" on:click={done} disabled={!canAcknowledge}>
        {acknowledgeLabel($t('vault.recovery.done'), secondsLeft)}
      </button>
    </div>
  </div>
</div>

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.6);
    /* Above every other dialog: nothing may be opened on top of the one screen that shows the key. */
    z-index: 3000;
  }

  .dialog {
    display: flex;
    flex-direction: column;
    gap: 12px;
    width: 440px;
    max-width: calc(100vw - 32px);
    padding: 20px;
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 6px;
  }

  h2 {
    margin: 0;
    font-size: 16px;
    font-weight: 600;
    color: var(--text-bright);
  }

  .subtitle {
    margin: 0;
    font-size: 12px;
    line-height: 1.45;
    color: var(--text-secondary);
  }

  .key {
    margin: 0;
    padding: 12px;
    background: var(--bg-input);
    border: 1px solid var(--border-color);
    border-radius: 4px;
    font-family: var(--font-mono);
    font-size: 14px;
    /* The groups are already dash-separated; letter-spacing keeps a hand-copied character from
       blurring into its neighbour. */
    letter-spacing: 0.5px;
    line-height: 1.6;
    text-align: center;
    color: var(--text-bright);
    user-select: all;
    overflow-wrap: anywhere;
  }

  .warning {
    display: flex;
    gap: 8px;
    align-items: flex-start;
    font-size: 11px;
    line-height: 1.45;
    color: var(--warning);
  }

  .warning :global(svg) {
    flex-shrink: 0;
    margin-top: 1px;
  }

  .error {
    margin: 0;
    /* Holds its line while empty so the buttons never jump when a save fails. */
    min-height: 15px;
    font-size: 11px;
    color: var(--danger);
  }

  .actions {
    display: flex;
    gap: 8px;
    justify-content: flex-end;
  }

  .actions button {
    display: inline-flex;
    gap: 6px;
    align-items: center;
    padding: 6px 14px;
    font-size: 13px;
  }

  .done {
    /* Keeps its width as the counter ticks from two digits to one and then vanishes. */
    min-width: 96px;
    justify-content: center;
  }
</style>
