<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import Modal from '../Modal.svelte';
  import type { PendingKey } from '../../api/keys';

  export let show = false;
  export let pending: PendingKey[] = [];
  export let busy = false;
  export let error = '';

  const dispatch = createEventDispatcher();

  let answers: Record<string, string> = {};
  let skipped: Record<string, boolean> = {};

  $: remaining = pending.filter((key) => !skipped[key.id] && !answers[key.id]).length;

  function submit() {
    const supplied: Record<string, string> = {};
    for (const key of pending) {
      if (!skipped[key.id] && answers[key.id]) supplied[key.id] = answers[key.id];
    }
    dispatch('submit', supplied);
    answers = {};
  }
</script>

<Modal title="Upgrade your vault" {show} on:close={() => dispatch('close')}>
  <p class="intro">
    Your keys are being moved to a stronger storage format. A copy of the vault as it is now is
    saved beside it first, and nothing is deleted.
  </p>

  {#if pending.length === 0}
    <p class="intro">Nothing needs a passphrase. This will take a moment.</p>
  {:else}
    <p class="intro">
      These keys have their own passphrase. Enter each one so it can be re-encrypted. If you cannot
      remember one, skip it — the key keeps working exactly as before and you can finish it later
      from the key manager.
    </p>

    <ul class="keys">
      {#each pending as key (key.id)}
        <li class:skipped={skipped[key.id]}>
          <div class="row">
            <span class="name">{key.comment || key.id}</span>
            <label class="skip">
              <input type="checkbox" bind:checked={skipped[key.id]} />
              Skip
            </label>
          </div>
          {#if !skipped[key.id]}
            <input
              type="password"
              placeholder="Passphrase for this key"
              autocomplete="off"
              bind:value={answers[key.id]}
            />
          {/if}
        </li>
      {/each}
    </ul>
  {/if}

  {#if error}<p class="error">{error}</p>{/if}

  <div class="dialog-actions">
    <button on:click={() => dispatch('close')} disabled={busy}>Not now</button>
    <button class="primary" on:click={submit} disabled={busy}>
      {busy ? 'Upgrading…' : remaining > 0 ? `Upgrade, skipping ${remaining}` : 'Upgrade'}
    </button>
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

  .dialog-actions button:disabled {
    opacity: 0.45;
    cursor: not-allowed;
  }

  .intro {
    margin: 0 0 10px;
    font-size: 12px;
    line-height: 1.5;
    color: var(--text-secondary, #aaa);
  }

  .keys {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 10px;
    max-height: 320px;
    overflow-y: auto;
  }

  li.skipped {
    opacity: 0.55;
  }

  .row {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 10px;
  }

  .name {
    font-size: 13px;
    color: var(--text-primary, #ddd);
  }

  .skip {
    display: flex;
    gap: 5px;
    align-items: center;
    font-size: 11px;
    color: var(--text-secondary, #888);
  }

  input[type='password'] {
    width: 100%;
    margin-top: 4px;
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
