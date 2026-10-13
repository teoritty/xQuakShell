<script lang="ts">
  // The enabled account: where it syncs, how the last round went, and the password operations.
  // A password change and a reset both rotate the account keys; the other devices then sign in
  // again, which is why the panel offers "sign in again" whenever the status says so.
  import PasswordField from '../../vault/PasswordField.svelte';
  import { syncStatus } from '../../../stores/syncState';
  import {
    changeSyncPassword,
    disableSync,
    reloginSync,
    resetSyncPassword,
    syncNow,
  } from '../../../actions/syncActions';
  import { t } from '../../../i18n/messages';

  let busy = false;
  let error = '';
  let password = '';
  let oldPassword = '';
  let newPassword = '';
  let confirmDisable = false;

  $: needsSignIn = $syncStatus.state === 'stopped' && !!$syncStatus.message;

  async function run(fn: () => Promise<string>) {
    busy = true;
    error = await fn();
    busy = false;
    password = oldPassword = newPassword = '';
  }

  const relogin = () => run(() => reloginSync(password));
  const change = () => run(() => changeSyncPassword(oldPassword, newPassword));
  const reset = () => run(() => resetSyncPassword(newPassword));
  const now = () => run(() => syncNow());

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
  <h4>{$t('sync.account.title')}</h4>
  <p class="sync-meta">{$syncStatus.username} · {$syncStatus.serverUrl}</p>
  <p class="sync-meta">{$t('sync.account.fingerprint')} <span class="sync-fingerprint">{$syncStatus.fingerprint}</span></p>
  <p class="sync-meta">{$t('sync.state.' + $syncStatus.state)}</p>
  {#if $syncStatus.message}
    <p class="sync-error" role="status">{$syncStatus.message}</p>
  {/if}
  <div class="sync-actions">
    <button type="button" class="primary" on:click={now} disabled={busy || !$syncStatus.running}>{$t('sync.account.syncNow')}</button>
    <button type="button" class="secondary" on:click={disable} disabled={busy}>
      {confirmDisable ? $t('sync.account.disableConfirm') : $t('sync.account.disable')}
    </button>
  </div>
  <p class="sync-hint">{$t('sync.account.disableHint')}</p>
</div>

{#if needsSignIn}
  <div class="section">
    <h4>{$t('sync.relogin.title')}</h4>
    <PasswordField bind:value={password} ariaLabel={$t('sync.enable.password')} placeholder={$t('sync.enable.password')} disabled={busy} compact />
    <div class="sync-actions">
      <button type="button" class="primary" on:click={relogin} disabled={busy || !password}>{$t('sync.relogin.action')}</button>
    </div>
  </div>
{/if}

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
      <button type="button" class="primary" on:click={change} disabled={busy || !oldPassword || !newPassword}>{$t('sync.password.change')}</button>
    {/if}
    <button type="button" class="secondary" on:click={reset} disabled={busy || !newPassword}>{$t('sync.password.reset')}</button>
  </div>
  <p class="sync-hint">{$t('sync.password.hint')}</p>
  <p class="sync-error" role="alert">{error}</p>
</div>
