<script lang="ts">
  import { AlertTriangle } from 'lucide-svelte';
  import { t } from '../../i18n/messages';
</script>

<div class="warning" role="alert">
  <AlertTriangle size={16} strokeWidth={2} />
  <span>{$t('security.vault.create.noRecovery')}</span>
</div>

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
</style>
