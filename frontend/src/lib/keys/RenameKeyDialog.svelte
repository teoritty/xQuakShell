<script lang="ts">
  import { t } from '../../i18n/messages';
  import { createEventDispatcher } from 'svelte';
  import Modal from '../Modal.svelte';
  import type { StoredKey } from '../../api/keys';

  export let show = false;
  export let target: StoredKey | null = null;

  const dispatch = createEventDispatcher();

  let name = '';
  let error = '';

  // The field is filled from the key each time the dialog opens rather than once at construction,
  // so renaming a second key does not offer the first one's name.
  $: if (show && target) {
    name = target.comment;
    error = '';
  }

  function submit() {
    if (!name.trim()) {
      error = 'A key needs a name you can tell it apart by.';
      return;
    }
    dispatch('submit', name.trim());
  }
</script>

<Modal title={$t('keys.rename.title')} {show} on:close={() => dispatch('close')}>
  <label for="rename-key">{$t('keys.field.name')}</label>
  <!-- svelte-ignore a11y_autofocus -->
  <input id="rename-key" bind:value={name} autofocus on:keydown={(e) => e.key === 'Enter' && submit()} />
  {#if error}<p class="error">{error}</p>{/if}

  <div class="dialog-actions">
    <button on:click={() => dispatch('close')}>{$t('common.cancel')}</button>
    <button class="primary" on:click={submit}>{$t('common.save')}</button>
  </div>
</Modal>

<style>
  .dialog-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 14px;
  }


  label {
    display: block;
    margin-bottom: 4px;
    font-size: 12px;
    color: var(--text-secondary);
  }


  .error {
    margin: 8px 0 0;
    font-size: 12px;
    color: var(--danger);
  }

</style>
