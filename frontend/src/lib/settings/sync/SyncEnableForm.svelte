<script lang="ts">
  // Enabling sync in three steps. Nothing but a hello reaches the server before the person has
  // compared its fingerprint with the one the server's admin printed: the identity confirmed here
  // is pinned for good, and every later connection must prove it.
  import PasswordField from '../../vault/PasswordField.svelte';
  import { enableSync, joinSync, probeSyncServer } from '../../../actions/syncActions';
  import { t } from '../../../i18n/messages';

  let step: 'server' | 'confirm' | 'account' | 'join' | 'joined' = 'server';
  let serverUrl = '';
  let identityKey = '';
  let fingerprint = '';
  let register = false;
  let username = '';
  let password = '';
  let invite = '';
  let busy = false;
  let error = '';
  let backup = '';

  $: canSubmit = !!username.trim() && !!password && (!register || !!invite.trim()) && !busy;

  async function probe() {
    busy = true;
    error = '';
    const res = await probeSyncServer(serverUrl);
    busy = false;
    if (res.error || !res.probe) {
      error = res.error ?? '';
      return;
    }
    identityKey = res.probe.identityKey;
    fingerprint = res.probe.fingerprint;
    step = 'confirm';
  }

  async function submit() {
    if (!canSubmit) return;
    busy = true;
    const res = await enableSync({ serverUrl: serverUrl.trim(), identityKey, username: username.trim(), password, invite: invite.trim(), register });
    busy = false;
    password = '';
    error = res.error;
    if (!res.error && res.joinNeeded) step = 'join';
  }

  async function join(choice: 'merge' | 'replace') {
    busy = true;
    const res = await joinSync(choice);
    busy = false;
    error = res.error;
    if (!res.error) {
      backup = res.backup;
      step = 'joined';
    }
  }

  function restart() {
    step = 'server';
    identityKey = fingerprint = error = '';
  }
</script>

<div class="section">
  <h4>{$t('sync.enable.title')}</h4>
  {#if step === 'server'}
    <p class="sync-hint">{$t('sync.enable.serverHint')}</p>
    <input class="sync-field" type="url" placeholder="https://sync.example.com" bind:value={serverUrl} disabled={busy} />
    <div class="sync-actions">
      <button type="button" class="primary" on:click={probe} disabled={!serverUrl.trim() || busy}>{$t('sync.enable.connect')}</button>
    </div>
  {:else if step === 'confirm'}
    <p class="sync-hint">{$t('sync.enable.compare')}</p>
    <p class="sync-fingerprint">{fingerprint}</p>
    <div class="sync-actions">
      <button type="button" class="primary" on:click={() => (step = 'account')}>{$t('sync.enable.matches')}</button>
      <button type="button" class="secondary" on:click={restart}>{$t('sync.enable.differs')}</button>
    </div>
  {:else if step === 'account'}
    <label class="checkbox-row">
      <input type="checkbox" bind:checked={register} disabled={busy} />
      {$t('sync.enable.register')}
    </label>
    <input class="sync-field" placeholder={$t('sync.enable.username')} bind:value={username} disabled={busy} />
    <PasswordField bind:value={password} ariaLabel={$t('sync.enable.password')} placeholder={$t('sync.enable.password')} disabled={busy} compact />
    {#if register}
      <input class="sync-field" placeholder={$t('sync.enable.invite')} bind:value={invite} disabled={busy} />
    {/if}
    <p class="sync-hint">{$t('sync.enable.passwordHint')}</p>
    <div class="sync-actions">
      <button type="button" class="primary" on:click={submit} disabled={!canSubmit}>
        {register ? $t('sync.enable.createAccount') : $t('sync.enable.signIn')}
      </button>
      <button type="button" class="secondary" on:click={restart} disabled={busy}>{$t('sync.enable.back')}</button>
    </div>
  {:else if step === 'join'}
    <p class="sync-hint">{$t('sync.join.explain')}</p>
    <div class="sync-actions">
      <button type="button" class="primary" on:click={() => join('merge')} disabled={busy}>{$t('sync.join.merge')}</button>
      <button type="button" class="secondary" on:click={() => join('replace')} disabled={busy}>{$t('sync.join.replace')}</button>
    </div>
    <p class="sync-hint">{$t('sync.join.backupHint')}</p>
  {:else}
    <p class="sync-hint">{$t('sync.join.done', { backup })}</p>
  {/if}
  <p class="sync-error" role="alert">{error}</p>
</div>
