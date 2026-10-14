<script lang="ts">
  // The account at a glance, and whatever needs the person now. A stopped account says what to do
  // about it - sign in again after a password change elsewhere, let go of a device that was removed,
  // or treat the server as suspect - instead of one message for all three. A device that joined the
  // account is announced here until the person says it is theirs or removes it: that notice is how
  // a stolen sync password shows, so it does not fade after one sync round.
  import PasswordField from '../../vault/PasswordField.svelte';
  import { syncStatus } from '../../../stores/syncState';
  import {
    acknowledgeSyncDevices,
    disableSync,
    joinSync,
    reloginSync,
    revokeSyncDevice,
    syncNow,
  } from '../../../actions/syncActions';
  import { t } from '../../../i18n/messages';

  let busy = false;
  let error = '';
  let password = '';
  let removing = '';
  let confirmDisable = false;

  $: stop = $syncStatus.state === 'stopped' ? $syncStatus.stopReason ?? 'integrity' : '';
  $: host = hostOf($syncStatus.serverUrl ?? '');

  function hostOf(url: string): string {
    try {
      return new URL(url).host;
    } catch {
      return url;
    }
  }

  async function run(fn: () => Promise<string>) {
    busy = true;
    error = await fn();
    busy = false;
    password = '';
    if (!error) removing = '';
  }

  async function disable() {
    if (!confirmDisable) {
      confirmDisable = true;
      return;
    }
    confirmDisable = false;
    await run(() => disableSync());
  }

  const date = (ms: number) => (ms > 0 ? new Date(ms).toLocaleDateString() : '');
</script>

<div class="section">
  <h4>{$t('sync.account.title')}</h4>
  <div class="sync-status-row">
    <span class="sync-dot" class:ok={$syncStatus.state === 'idle'} class:bad={stop !== '' || $syncStatus.state === 'failed'}></span>
    <span>{$syncStatus.username} · {host}</span>
    <span class="sync-meta">{$t('sync.state.' + $syncStatus.state)}</span>
    <button type="button" class="secondary" on:click={() => run(() => syncNow())} disabled={busy || !$syncStatus.running}>{$t('sync.account.syncNow')}</button>
  </div>

  {#if $syncStatus.joinPending}
    <div class="sync-attention">
      <p>{$t('sync.join.explain')}</p>
      <div class="sync-actions">
        <button type="button" class="primary" on:click={() => run(async () => (await joinSync('merge')).error)} disabled={busy}>{$t('sync.join.merge')}</button>
        <button type="button" class="secondary" on:click={() => run(async () => (await joinSync('replace')).error)} disabled={busy}>{$t('sync.join.replace')}</button>
      </div>
    </div>
  {/if}

  {#if stop === 'signIn'}
    <div class="sync-attention" role="status">
      <p>{$syncStatus.message}</p>
      <PasswordField bind:value={password} ariaLabel={$t('sync.enable.password')} placeholder={$t('sync.enable.password')} disabled={busy} compact />
      <div class="sync-actions">
        <button type="button" class="primary" on:click={() => run(() => reloginSync(password))} disabled={busy || !password}>{$t('sync.relogin.action')}</button>
      </div>
    </div>
  {:else if stop === 'revoked'}
    <div class="sync-attention" role="status">
      <p>{$syncStatus.message}</p>
      <div class="sync-actions">
        <button type="button" class="danger" on:click={disable} disabled={busy}>
          {confirmDisable ? $t('sync.account.disableConfirm') : $t('sync.account.disable')}
        </button>
      </div>
    </div>
  {:else if stop === 'integrity'}
    <div class="sync-attention danger" role="alert">
      <p>{$syncStatus.message}</p>
      <p class="sync-hint">{$t('security.sync.stopped.integrityHint')}</p>
    </div>
  {:else if $syncStatus.state === 'failed' && $syncStatus.message}
    <p class="sync-meta" role="status">{$syncStatus.message}</p>
  {/if}

  {#each $syncStatus.newDevices ?? [] as d (d.id)}
    <div class="sync-attention" role="alert">
      <p>{$t('security.sync.newDevice.joined', { name: d.name, platform: d.platform, date: date(d.createdUnixMilli) })}</p>
      {#if removing === d.id}
        <p class="sync-hint">{$t('security.sync.newDevice.removeHint')}</p>
        <PasswordField bind:value={password} ariaLabel={$t('sync.enable.password')} placeholder={$t('sync.enable.password')} disabled={busy} compact />
        <div class="sync-actions">
          <button type="button" class="danger" on:click={() => run(() => revokeSyncDevice(d.id, password))} disabled={busy || !password}>{$t('sync.devices.removeConfirm')}</button>
          <button type="button" class="secondary" on:click={() => (removing = '')} disabled={busy}>{$t('sync.enable.back')}</button>
        </div>
      {:else}
        <div class="sync-actions">
          <button type="button" class="primary" on:click={() => run(() => acknowledgeSyncDevices([d.id]))} disabled={busy}>{$t('security.sync.newDevice.mine')}</button>
          <button type="button" class="danger" on:click={() => (removing = d.id)} disabled={busy}>{$t('security.sync.newDevice.notMine')}</button>
        </div>
      {/if}
    </div>
  {/each}
  <p class="sync-error" role="alert">{error}</p>
</div>
