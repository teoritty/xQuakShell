<script lang="ts">
  // The trash of the synced vault, and Shred now. Deleted items stay restorable for the vault's
  // retention window and are then purged everywhere. Shred now cannot be undone, so it takes the
  // master password and a second click, and says what changing the sync password adds to it.
  import { onMount } from 'svelte';
  import PasswordField from '../../vault/PasswordField.svelte';
  import { listSyncTrashRpc, type SyncTrashItem } from '../../../api/sync';
  import { errorMessage, restoreSyncTrash, shredSyncNow } from '../../../actions/syncActions';
  import { t } from '../../../i18n/messages';

  let items: SyncTrashItem[] = [];
  let masterPassword = '';
  let confirming = false;
  let busy = false;
  let error = '';

  onMount(load);

  async function load() {
    try {
      items = await listSyncTrashRpc();
      error = '';
    } catch (e) {
      error = errorMessage(e);
    }
  }

  async function restore(record: string) {
    busy = true;
    error = await restoreSyncTrash(record);
    busy = false;
    await load();
  }

  async function shred() {
    if (!confirming) {
      confirming = true;
      return;
    }
    busy = true;
    error = await shredSyncNow(masterPassword);
    busy = false;
    confirming = false;
    masterPassword = '';
    await load();
  }

  const date = (ms: number) => new Date(ms).toLocaleDateString();
</script>

<div class="section">
  <h4>{$t('sync.trash.title')}</h4>
  {#if items.length === 0}
    <p class="sync-hint">{$t('sync.trash.none')}</p>
  {:else}
    <ul class="sync-list">
      {#each items as it (it.record)}
        <li>
          <div>{it.restorable ? `${it.type} ${it.key}` : $t('sync.trash.unreadable')}</div>
          <div class="sync-meta">{$t('sync.trash.purgedOn', { date: date(it.purgeUnixMilli) })}</div>
          {#if it.restorable}
            <div class="sync-actions">
              <button type="button" class="secondary" on:click={() => restore(it.record)} disabled={busy}>{$t('sync.trash.restore')}</button>
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
</div>

<div class="section">
  <h4>{$t('sync.shred.title')}</h4>
  <p class="sync-hint">{$t('sync.shred.hint')}</p>
  <PasswordField bind:value={masterPassword} ariaLabel={$t('sync.held.masterPassword')} placeholder={$t('sync.held.masterPassword')} disabled={busy} compact />
  <div class="sync-actions">
    <button type="button" class="danger" on:click={shred} disabled={busy || !masterPassword}>
      {confirming ? $t('sync.shred.confirm') : $t('sync.shred.action')}
    </button>
  </div>
  <p class="sync-error" role="alert">{error}</p>
</div>
