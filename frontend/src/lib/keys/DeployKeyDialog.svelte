<script lang="ts">
  import { t } from '../../i18n/messages';
  import { createEventDispatcher } from 'svelte';
  import Modal from '../Modal.svelte';
  import type { Session } from '../../stores/appState';
  import type { StoredKey } from '../../api/keys';

  export let show = false;
  export let target: StoredKey | null = null;
  export let sessions: Session[] = [];
  export let error = '';
  // The outcome is shown in the dialog rather than through the error toast: publishing succeeds
  // far more often than it fails, and reporting a success on the error channel is a lie about it.
  export let notice = '';
  export let busy = false;

  const dispatch = createEventDispatcher();

  let sessionId = '';

  // Only connected sessions can be published to: the key is appended over that session's own SFTP
  // channel, so there is nothing to write through until the handshake has finished.
  $: ready = sessions.filter((s) => s.state === 'ready');
  $: if (show && !sessionId && ready.length > 0) sessionId = ready[0].sessionId;
</script>

<Modal title={$t('keys.deploy.title', { name: target?.comment || $t('keys.fallbackName') })} {show} on:close={() => dispatch('close')}>
  {#if ready.length === 0}
    <p class="explain">
      Open a connection first. The key is added over a session you are already logged in to, so
      nothing new is asked for your password.
    </p>
  {:else}
    <label for="deploy-session">{$t('keys.deploy.addTo')}</label>
    <select id="deploy-session" bind:value={sessionId}>
      {#each ready as session (session.sessionId)}
        <option value={session.sessionId}>{session.connectionName}</option>
      {/each}
    </select>
    <p class="explain">
      Appends the public key to <code>~/.ssh/authorized_keys</code> for the user of that session,
      keeping every key already there. If it is already authorised, nothing changes.
    </p>
  {/if}

  {#if notice}<p class="notice">{notice}</p>{/if}
  {#if error}<p class="error">{error}</p>{/if}

  <div class="dialog-actions">
    <button on:click={() => dispatch('close')}>{$t('common.cancel')}</button>
    <button class="primary" on:click={() => dispatch('submit', sessionId)} disabled={busy || ready.length === 0}>
      {busy ? $t('keys.deploy.busy') : notice ? $t('keys.deploy.again') : $t('keys.deploy.submit')}
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



  label {
    display: block;
    margin-bottom: 4px;
    font-size: 12px;
    color: var(--text-secondary);
  }


  .explain {
    margin: 8px 0 0;
    font-size: 11px;
    line-height: 1.5;
    color: var(--text-secondary);
  }

  code {
    font-family: var(--font-mono);
  }

  .error {
    margin: 8px 0 0;
    font-size: 12px;
    color: var(--danger);
  }

  .notice {
    margin: 10px 0 0;
    padding: 8px 10px;
    border-radius: 4px;
    background: var(--accent-muted);
    color: var(--text-bright);
    font-size: 12px;
    line-height: 1.45;
  }

</style>
