<script lang="ts">
  import { t } from '../../i18n/messages';

  // Sits directly above the submit button, which is the moment it has to be answered rather than
  // the moment the warning is read. A checkbox at the top of a form is scrolled past on the way to
  // the password fields and ticked from memory on the way back down.
  export let acknowledged = false;
  export let disabled = false;
</script>

<label class="understand">
  <input type="checkbox" bind:checked={acknowledged} {disabled} />
  <span>{$t('security.vault.create.understand')}</span>
</label>

<style>
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
    /* Aligns the box with the first line of the label rather than the middle of the block. */
    margin: 1px 0 0;
    cursor: pointer;
  }
</style>
