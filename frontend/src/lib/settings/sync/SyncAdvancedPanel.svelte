<script lang="ts">
  // What most people never need and a careful one wants within reach: both fingerprints, the sync
  // password, and turning sync off on this device. A password change and a reset both rotate the
  // account keys; the other devices then ask to sign in again.
  import PasswordField from '../../vault/PasswordField.svelte';
  import { syncStatus } from '../../../stores/syncState';
  import { changeSyncPassword, disableSync, resetSyncPassword } from '../../../actions/syncActions';
  import { t } from '../../../i18n/messages';

  let busy = false;
  let error = '';
  let oldPassword = '';
  let newPassword = '';
  let confirmDisable = false;

  async function run(fn: () => Promise<string>) {
    busy = true;
    error = await fn();
    busy = false;
    oldPassword = newPassword = '';
  }

  async function disable() {
    if (!confirmDisable) {
      confirmDisable = true;
      return;
    }
    confirmDisable = false;
    await run(() => disableSync());
  }
</script>

<div class="section">
  <h4>{$t('sync.advanced.identity')}</h4>
  <p class="sync-meta">{$syncStatus.serverUrl}</p>
  <p class="sync-meta">{$t('sync.account.fingerprint')} <span class="sync-fingerprint">{$syncStatus.fingerprint}</span></p>
  {#if $syncStatus.accountFingerprint}
    <p class="sync-meta">{$t('sync.account.own')} <span class="sync-fingerprint">{$syncStatus.accountFingerprint}</span></p>
  {/if}
</div>

<div class="section">
  <h4>{$syncStatus.rekeyPending ? $t('sync.password.finishTitle') : $t('sync.password.title')}</h4>
  {#if $syncStatus.rekeyPending}
    <p class="sync-error" role="status">{$t('sync.password.rekeyPending')}</p>
  {:else}
    <PasswordField bind:value={oldPassword} ariaLabel={$t('sync.password.old')} placeholder={$t('sync.password.old')} disabled={busy} compact />
  {/if}
  <PasswordField bind:value={newPassword} ariaLabel={$t('sync.password.new')} placeholder={$t('sync.password.new')} disabled={busy} compact />
  <div class="sync-actions">
    {#if !$syncStatus.rekeyPending}
      <button type="button" class="primary" on:click={() => run(() => changeSyncPassword(oldPassword, newPassword))} disabled={busy || !oldPassword || !newPassword}>{$t('sync.password.change')}</button>
    {/if}
    <button type="button" class="secondary" on:click={() => run(() => resetSyncPassword(newPassword))} disabled={busy || !newPassword}>{$t('sync.password.reset')}</button>
  </div>
  <p class="sync-hint">{$t('sync.password.hint')}</p>
</div>

<div class="section">
  <h4>{$t('sync.advanced.thisDevice')}</h4>
  <div class="sync-actions">
    <button type="button" class="secondary" on:click={disable} disabled={busy}>
      {confirmDisable ? $t('sync.account.disableConfirm') : $t('sync.account.disable')}
    </button>
  </div>
  <p class="sync-hint">{$t('sync.account.disableHint')}</p>
  <p class="sync-error" role="alert">{error}</p>
</div>
