<script lang="ts">
  import { t } from '../../i18n/messages';
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

<Modal title={$t('keys.migrate.title')} {show} on:close={() => dispatch('close')}>
  <p class="intro">{$t('security.keys.migrate.intro')}</p>

  {#if pending.length === 0}
    <p class="intro">{$t('keys.migrate.nothingNeeded')}</p>
  {:else}
    <p class="intro">{$t('keys.migrate.needPassphrases')}</p>

    <ul class="keys">
      {#each pending as key (key.id)}
        <li class:skipped={skipped[key.id]}>
          <div class="row">
            <span class="name">{key.comment || key.id}</span>
            <label class="skip">
              <input type="checkbox" bind:checked={skipped[key.id]} />
              {$t('keys.migrate.skip')}
            </label>
          </div>
          {#if !skipped[key.id]}
            <input
              type="password"
              placeholder={$t('keys.migrate.passphrasePlaceholder')}
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
    <button on:click={() => dispatch('close')} disabled={busy}>{$t('keys.migrate.notNow')}</button>
    <button class="primary" on:click={submit} disabled={busy}>
      {busy
        ? $t('keys.migrate.busy')
        : remaining > 0
          ? $t('keys.migrate.skipping', { count: remaining })
          : $t('keys.migrate.submit')}
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



  .intro {
    margin: 0 0 10px;
    font-size: 12px;
    line-height: 1.5;
    color: var(--text-secondary);
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
    color: var(--text-primary);
  }

  .skip {
    display: flex;
    gap: 5px;
    align-items: center;
    font-size: 11px;
    color: var(--text-secondary);
  }


  .error {
    margin: 8px 0 0;
    font-size: 12px;
    color: var(--danger);
  }

</style>
