<script lang="ts">
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

<Modal title="Publish {target?.comment || 'key'}" {show} on:close={() => dispatch('close')}>
  {#if ready.length === 0}
    <p class="explain">
      Open a connection first. The key is added over a session you are already logged in to, so
      nothing new is asked for your password.
    </p>
  {:else}
    <label for="deploy-session">Add to</label>
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
    <button on:click={() => dispatch('close')}>Cancel</button>
    <button class="primary" on:click={() => dispatch('submit', sessionId)} disabled={busy || ready.length === 0}>
      {busy ? 'Publishing…' : notice ? 'Publish again' : 'Publish'}
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

  label {
    display: block;
    margin-bottom: 4px;
    font-size: 12px;
    color: var(--text-secondary, #888);
  }

  select {
    width: 100%;
    padding: 6px 8px;
    font-size: 13px;
    border-radius: 4px;
    border: 1px solid var(--border, rgba(255, 255, 255, 0.14));
    background: var(--bg-input, rgba(0, 0, 0, 0.25));
    color: var(--text-primary, #ddd);
  }

  .explain {
    margin: 8px 0 0;
    font-size: 11px;
    line-height: 1.5;
    color: var(--text-secondary, #888);
  }

  code {
    font-family: var(--font-mono, monospace);
  }

  .error {
    margin: 8px 0 0;
    font-size: 12px;
    color: var(--danger, #ff6b6b);
  }

  .notice {
    margin: 10px 0 0;
    padding: 8px 10px;
    border-radius: 4px;
    background: var(--ok-bg, rgba(74, 158, 255, 0.14));
    color: var(--ok-fg, #7ab8ff);
    font-size: 12px;
    line-height: 1.45;
  }

  button.primary {
    background: var(--accent, #4a9eff);
    color: #fff;
    border-color: transparent;
  }
</style>
