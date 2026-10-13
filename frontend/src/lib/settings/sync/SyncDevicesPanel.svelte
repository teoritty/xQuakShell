<script lang="ts">
  // The account's devices. Removing one asks for the sync password: the account keys rotate and
  // the new bundle is sealed under it, so the removed device can never fetch them again.
  import { onMount } from 'svelte';
  import PasswordField from '../../vault/PasswordField.svelte';
  import { listSyncDevicesRpc, type SyncDevice } from '../../../api/sync';
  import { errorMessage, revokeSyncDevice } from '../../../actions/syncActions';
  import { t } from '../../../i18n/messages';

  let devices: SyncDevice[] = [];
  let removing = '';
  let password = '';
  let busy = false;
  let error = '';

  onMount(load);

  async function load() {
    try {
      devices = await listSyncDevicesRpc();
      error = '';
    } catch (e) {
      error = errorMessage(e);
    }
  }

  async function revoke(id: string) {
    busy = true;
    error = await revokeSyncDevice(id, password);
    busy = false;
    password = '';
    if (!error) {
      removing = '';
      await load();
    }
  }

  function created(ms: number): string {
    return ms > 0 ? new Date(ms).toLocaleDateString() : '';
  }
</script>

<div class="section">
  <h4>{$t('sync.devices.title')}</h4>
  <ul class="sync-list">
    {#each devices as d (d.id)}
      <li>
        <div>
          {d.name}
          {#if d.current}<span class="sync-meta">· {$t('sync.devices.this')}</span>{/if}
          {#if d.revoked}<span class="sync-meta">· {$t('sync.devices.revoked')}</span>{/if}
        </div>
        <div class="sync-meta">{d.platform} · {created(d.createdUnixMilli)}</div>
        {#if !d.current && !d.revoked}
          {#if removing === d.id}
            <PasswordField bind:value={password} ariaLabel={$t('sync.enable.password')} placeholder={$t('sync.enable.password')} disabled={busy} compact />
            <div class="sync-actions">
              <button type="button" class="primary" on:click={() => revoke(d.id)} disabled={busy || !password}>{$t('sync.devices.removeConfirm')}</button>
              <button type="button" class="secondary" on:click={() => (removing = '')} disabled={busy}>{$t('sync.enable.back')}</button>
            </div>
          {:else}
            <button type="button" class="secondary" on:click={() => (removing = d.id)}>{$t('sync.devices.remove')}</button>
          {/if}
        {/if}
      </li>
    {/each}
  </ul>
  <p class="sync-hint">{$t('sync.devices.hint')}</p>
  <p class="sync-error" role="alert">{error}</p>
</div>
