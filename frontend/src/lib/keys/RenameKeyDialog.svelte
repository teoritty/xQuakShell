<script lang="ts">
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

<Modal title="Rename key" {show} on:close={() => dispatch('close')}>
  <label for="rename-key">Name</label>
  <!-- svelte-ignore a11y_autofocus -->
  <input id="rename-key" bind:value={name} autofocus on:keydown={(e) => e.key === 'Enter' && submit()} />
  {#if error}<p class="error">{error}</p>{/if}

  <div class="dialog-actions">
    <button on:click={() => dispatch('close')}>Cancel</button>
    <button class="primary" on:click={submit}>Save</button>
  </div>
</Modal>

<style>
  .dialog-actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
    margin-top: 14px;
  }

  .dialog-actions button {
    padding: 6px 14px;
    font-size: 12px;
    border-radius: 4px;
    border: 1px solid var(--border, rgba(255, 255, 255, 0.14));
    background: var(--bg-button, rgba(255, 255, 255, 0.06));
    color: var(--text-primary, #ddd);
    cursor: pointer;
  }

  label {
    display: block;
    margin-bottom: 4px;
    font-size: 12px;
    color: var(--text-secondary, #888);
  }

  input {
    width: 100%;
    padding: 6px 8px;
    font-size: 13px;
    border-radius: 4px;
    border: 1px solid var(--border, rgba(255, 255, 255, 0.14));
    background: var(--bg-input, rgba(0, 0, 0, 0.25));
    color: var(--text-primary, #ddd);
  }

  .error {
    margin: 8px 0 0;
    font-size: 12px;
    color: var(--danger, #ff6b6b);
  }

  button.primary {
    background: var(--accent, #4a9eff);
    color: #fff;
    border-color: transparent;
  }
</style>
