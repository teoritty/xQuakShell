<script lang="ts">
  // What another device changed that this one did not simply apply. A held change weakens this
  // device - plugin trust, the lockout, a key's permissions, a replaced host key - and waits for
  // the master password; a refused one is never applied. Rejecting either writes this device's
  // value back for every device. Conflicts are credential fields two devices set differently.
  import { onMount } from 'svelte';
  import PasswordField from '../../vault/PasswordField.svelte';
  import { listSyncConflictsRpc, listSyncHeldRpc, type SyncConflict, type SyncHeld } from '../../../api/sync';
  import { confirmSyncHeld, errorMessage, rejectSyncHeld, resolveSyncConflict } from '../../../actions/syncActions';
  import { syncStatus } from '../../../stores/syncState';
  import { t } from '../../../i18n/messages';

  let held: SyncHeld[] = [];
  let conflicts: SyncConflict[] = [];
  let masterPassword = '';
  let busy = false;
  let error = '';

  onMount(load);
  // A round that held or refused something refreshes the list without the page reopening.
  $: if ($syncStatus.held || $syncStatus.quarantined) void load();

  async function load() {
    try {
      [held, conflicts] = await Promise.all([listSyncHeldRpc(), listSyncConflictsRpc()]);
      error = '';
    } catch (e) {
      error = errorMessage(e);
    }
  }

  async function act(fn: () => Promise<string>) {
    busy = true;
    error = await fn();
    busy = false;
    masterPassword = '';
    await load();
  }
</script>

<div class="section">
  <h4>{$t('sync.held.title')}</h4>
  {#if held.length === 0}
    <p class="sync-hint">{$t('sync.held.none')}</p>
  {:else}
    <PasswordField bind:value={masterPassword} ariaLabel={$t('sync.held.masterPassword')} placeholder={$t('sync.held.masterPassword')} disabled={busy} compact />
    <ul class="sync-list">
      {#each held as h (h.record)}
        <li>
          <div>{h.type || $t('sync.held.unreadable')} {h.key}</div>
          <div class="sync-meta">{h.reason}</div>
          <div class="sync-actions">
            {#if h.pending}
              <button type="button" class="primary" on:click={() => act(() => confirmSyncHeld(h.record, masterPassword))} disabled={busy || !masterPassword}>{$t('sync.held.accept')}</button>
            {/if}
            <button type="button" class="secondary" on:click={() => act(() => rejectSyncHeld(h.record))} disabled={busy}>{$t('sync.held.reject')}</button>
          </div>
        </li>
      {/each}
    </ul>
    <p class="sync-hint">{$t('sync.held.hint')}</p>
  {/if}
</div>

<div class="section">
  <h4>{$t('sync.conflicts.title')}</h4>
  {#if conflicts.length === 0}
    <p class="sync-hint">{$t('sync.conflicts.none')}</p>
  {:else}
    <ul class="sync-list">
      {#each conflicts as c (c.id)}
        <li>
          <div>{c.recordType} {c.itemKey} · {c.path.join('.')}</div>
          {#if c.secret}
            <div class="sync-meta">{$t('sync.conflicts.secret')}</div>
          {:else}
            <div class="sync-meta">{$t('sync.conflicts.current')} {c.remote} · {$t('sync.conflicts.other')} {c.local}</div>
          {/if}
          <div class="sync-actions">
            <button type="button" class="primary" on:click={() => act(() => resolveSyncConflict(c.id, false))} disabled={busy}>{$t('sync.conflicts.keep')}</button>
            <button type="button" class="secondary" on:click={() => act(() => resolveSyncConflict(c.id, true))} disabled={busy}>{$t('sync.conflicts.useOther')}</button>
          </div>
        </li>
      {/each}
    </ul>
  {/if}
  <p class="sync-error" role="alert">{error}</p>
</div>
