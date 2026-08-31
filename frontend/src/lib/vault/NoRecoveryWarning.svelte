<script lang="ts">
  import { AlertTriangle } from 'lucide-svelte';
  import { t } from '../../i18n/messages';

  // Bound by the form, which refuses to submit until it is true. The acknowledgement is the whole
  // reason the banner is not just text: a warning nobody has to touch is a warning nobody reads.
  export let acknowledged = false;
  export let disabled = false;
</script>

<div class="warning" role="alert">
  <AlertTriangle size={16} strokeWidth={2} />
  <span>{$t('security.vault.create.noRecovery')}</span>
</div>

<label class="understand">
  <input type="checkbox" bind:checked={acknowledged} {disabled} />
  <span>{$t('security.vault.create.understand')}</span>
</label>

<style>
  .warning {
    display: flex;
    gap: 8px;
    align-items: flex-start;
    padding: 10px 12px;
    /* The same fill and border the vault error banner uses, so the two read as one severity. */
    background: rgba(197, 80, 80, 0.15);
    border: 1px solid var(--danger);
    border-radius: 4px;
    color: var(--danger);
    font-size: 12px;
    line-height: 1.45;
  }

  .warning :global(svg) {
    flex-shrink: 0;
    /* Aligns the icon with the first line of text rather than the middle of the block. */
    margin-top: 1px;
  }

  .understand {
    display: flex;
    gap: 8px;
    align-items: flex-start;
    font-size: 12px;
    line-height: 1.4;
    color: var(--text-primary);
    cursor: pointer;
  }

  .understand input {
    flex-shrink: 0;
    margin: 1px 0 0;
    cursor: pointer;
  }
</style>
