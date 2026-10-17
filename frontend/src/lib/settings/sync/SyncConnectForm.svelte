<script lang="ts">
  // Turning sync on, on one screen. The address may be the connect link the server's admin
  // handed out; its fingerprint then vouches for the server and nothing has to be compared by
  // hand. With a bare address the fingerprint appears as soon as the address does, and the person
  // ticks that it matches the admin's. Either way nothing but a hello reaches the server before
  // that - the identity confirmed here is pinned for good, and every later connection must prove it.
  //
  // An invite code makes a new account; without one this signs in to an existing account. When
  // the device and the account both hold data they are merged, after a backup; replacing this
  // device's data instead is an advanced choice.
  import PasswordField from '../../vault/PasswordField.svelte';
  import { connectSync, identityVouched, probeSyncServer, type ConnectSyncForm } from '../../../actions/syncActions';
  import { parseSyncLink, normalizeFingerprint } from './syncLink';
  import type { SyncServerProbe } from '../../../api/sync';
  import { t } from '../../../i18n/messages';

  export let advanced = false;

  let form: ConnectSyncForm = { address: '', username: '', password: '', invite: '', expectedFingerprint: '', confirmed: false, replace: false };
  let probe: (SyncServerProbe & { url: string }) | null = null;
  let probing = '';
  let probeError = '';
  let busy = false;
  let error = '';
  let timer: ReturnType<typeof setTimeout> | undefined;
  let pending = '';

  $: link = parseSyncLink(form.address);
  $: vouchedBy = link?.fingerprint ? 'link' : normalizeFingerprint(form.expectedFingerprint) ? 'pasted' : '';
  $: identityProblem = probe ? identityVouched(form, probe) : '';
  $: canConnect = !!probe && !busy && !!form.username.trim() && !!form.password && !identityProblem;
  // The probe follows the address as it is typed, after a pause, so the fingerprint is on screen by
  // the time the rest of the form is filled in. A new address forgets the confirmation of the old.
  $: scheduleProbe(link?.url ?? '');

  // Runs on every keystroke anywhere in the form, because `link` depends on the whole form: only a
  // different address may restart the probe, and a probe already under way for it is left to finish.
  function scheduleProbe(url: string) {
    if (url === pending || url === probing || (probe && probe.url === url)) return;
    clearTimeout(timer);
    pending = probing = '';
    if (probe) {
      probe = null;
      form.confirmed = false;
    }
    probeError = '';
    if (!url) return;
    pending = url;
    timer = setTimeout(() => {
      pending = '';
      void runProbe(url);
    }, 500);
  }

  async function runProbe(url: string) {
    probing = url;
    probeError = '';
    const res = await probeSyncServer(url);
    if (probing !== url) return;
    probing = '';
    if (res.probe) probe = { ...res.probe, url };
    else probeError = res.error ?? '';
  }

  async function submit() {
    if (!canConnect || !probe) return;
    busy = true;
    error = await connectSync(form, probe);
    busy = false;
    form.password = '';
  }
</script>

<div class="section">
  <h4>{$t('sync.enable.title')}</h4>
  <p class="sync-hint">{$t('sync.connect.hint')}</p>
  <!-- novalidate, and a text field for the address: the browser's own URL check refused a host
       typed without https:// and blocked the submit without a word, where parseSyncLink accepts it. -->
  <form class="sync-form" novalidate on:submit|preventDefault={submit}>
    <label class="sync-label" for="sync-address">{$t('sync.connect.address')}</label>
    <input id="sync-address" class="sync-field" type="text" inputmode="url" spellcheck="false" placeholder="https://sync.example.org:8443" bind:value={form.address} disabled={busy} autocomplete="url" />
    {#if form.address.trim() && !link}
      <p class="sync-error">{$t('sync.connect.badAddress')}</p>
    {:else if probing}
      <p class="sync-meta">{$t('sync.connect.probing')}</p>
    {:else if probeError}
      <div class="sync-actions">
        <span class="sync-error" role="alert">{probeError}</span>
        <button type="button" class="secondary" on:click={() => link && runProbe(link.url)} disabled={busy}>{$t('sync.connect.retry')}</button>
      </div>
    {:else if probe}
      <div class="sync-identity" class:mismatch={identityProblem && vouchedBy}>
        <span class="sync-meta">{$t('sync.account.fingerprint')}</span>
        <span class="sync-fingerprint">{probe.fingerprint}</span>
        {#if vouchedBy && !identityProblem}
          <span class="sync-ok">{$t(vouchedBy === 'link' ? 'security.sync.connect.matchesLink' : 'security.sync.connect.matchesPasted')}</span>
        {:else if vouchedBy}
          <span class="sync-error" role="alert">{identityProblem}</span>
        {:else}
          <label class="checkbox-row">
            <input type="checkbox" bind:checked={form.confirmed} disabled={busy} />
            {$t('security.sync.connect.confirm')}
          </label>
        {/if}
      </div>
    {/if}

    <label class="sync-label" for="sync-user">{$t('sync.enable.username')}</label>
    <input id="sync-user" class="sync-field" bind:value={form.username} disabled={busy} autocomplete="username" />
    <span class="sync-label">{$t('sync.enable.password')}</span>
    <PasswordField bind:value={form.password} ariaLabel={$t('sync.enable.password')} placeholder={$t('sync.enable.password')} disabled={busy} compact />
    <label class="sync-label" for="sync-invite">{$t('sync.connect.invite')}</label>
    <input id="sync-invite" class="sync-field" bind:value={form.invite} disabled={busy} placeholder={$t('sync.connect.invitePlaceholder')} />

    {#if advanced}
      <label class="sync-label" for="sync-expected">{$t('sync.connect.expected')}</label>
      <input id="sync-expected" class="sync-field" bind:value={form.expectedFingerprint} disabled={busy} placeholder="04512 99031 …" />
      <label class="checkbox-row">
        <input type="checkbox" bind:checked={form.replace} disabled={busy} />
        {$t('sync.connect.replace')}
      </label>
    {/if}

    <p class="sync-hint">{$t('sync.enable.passwordHint')}</p>
    <div class="sync-actions">
      <button type="submit" class="primary" disabled={!canConnect}>
        {busy ? $t('sync.connect.connecting') : form.invite.trim() ? $t('sync.enable.createAccount') : $t('sync.connect.action')}
      </button>
    </div>
    <p class="sync-error" role="alert">{error}</p>
  </form>
</div>
