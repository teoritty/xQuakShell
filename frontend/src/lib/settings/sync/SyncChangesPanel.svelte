<script lang="ts">
  // What another device changed that this one did not simply apply. A held change weakens this
  // device - plugin trust, the lockout, a key's permissions, a replaced host key - and waits for
  // the master password; a refused one is never applied. Rejecting either writes this device's
  // value back for every device. Conflicts are credential fields two devices set differently.
  // Nothing shows while nothing waits: an empty list is not something to read every time.
  import PasswordField from '../../vault/PasswordField.svelte';
  import { listSyncConflictsRpc, listSyncHeldRpc, type SyncConflict, type SyncHeld } from '../../../api/sync';
  import { confirmSyncHeld, errorMessage, rejectSyncHeld, resolveSyncConflict } from '../../../actions/syncActions';
  import { syncRevision, syncStatus } from '../../../stores/syncState';
  import { conflictFieldLabel, itemLabel } from './syncLabels';
  import { t } from '../../../i18n/messages';

  let held: SyncHeld[] = [];
  let conflicts: SyncConflict[] = [];
  let masterPassword = '';
  let busy = false;
  let error = '';

  // Reloads on open, after every round that held or refused something, and whenever sync changed
  // the vault - a conflict resolved on another device disappears here too.
  $: void load($syncRevision, $syncStatus.held, $syncStatus.quarantined);

  async function load(..._deps: unknown[]) {
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

{#if held.length > 0}
  <div class="section">
    <h4>{$t('sync.held.title')}</h4>
    <p class="sync-hint">{$t('sync.held.hint')}</p>
    <PasswordField bind:value={masterPassword} ariaLabel={$t('sync.held.masterPassword')} placeholder={$t('sync.held.masterPassword')} disabled={busy} compact />
    <ul class="sync-list">
      {#each held as h (h.record)}
        <li>
          <div>{h.type ? itemLabel(h.type, h.name, $t) : $t('sync.held.unreadable')}</div>
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
  </div>
{/if}

{#if conflicts.length > 0}
  <div class="section">
    <h4>{$t('sync.conflicts.title')}</h4>
    <ul class="sync-list">
      {#each conflicts as c (c.id)}
        <li>
          <div>{itemLabel(c.recordType, c.itemName, $t)} · {conflictFieldLabel(c.path, c.pathNames, $t)}</div>
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
  </div>
{/if}

{#if error}
  <p class="sync-error" role="alert">{error}</p>
{/if}
