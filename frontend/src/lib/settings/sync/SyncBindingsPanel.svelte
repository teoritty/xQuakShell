<script lang="ts">
  // This member's own credentials for team connections whose users take one from each member.
  // A binding another member suspended by repointing the connection shows who did it and waits for
  // a confirmation of the new address before the credential is used again.
  import { onMount } from 'svelte';
  import { listCredentialBindingsRpc, type CredentialBinding } from '../../../api/team';
  import { bindCredential, confirmBinding } from '../../../actions/teamActions';
  import { errorMessage } from '../../../actions/syncActions';
  import { fetchKeys, type StoredKey } from '../../../api/keys';
  import { connections } from '../../../stores/appState';
  import { t } from '../../../i18n/messages';

  let bindings: CredentialBinding[] = [];
  let keys: StoredKey[] = [];
  let target = '';
  let key = '';
  let busy = false;
  let error = '';

  // Every user of a team connection that takes its credential from each member.
  $: open = $connections.flatMap((c) =>
    c.vaultId ? (c.users ?? []).filter((u) => u.credentialSource === 'personal').map((u) => ({ c, u })) : [],
  );

  onMount(load);

  async function load() {
    try {
      [bindings, keys] = await Promise.all([listCredentialBindingsRpc(), fetchKeys()]);
      error = '';
    } catch (e) {
      error = errorMessage(e);
    }
  }

  function label(b: CredentialBinding): string {
    const c = $connections.find((x) => x.id === b.connection && x.vaultId === b.vault);
    const u = c?.users?.find((x) => x.id === b.user);
    return c ? `${c.name} · ${u?.username ?? b.user}` : b.connection;
  }

  async function act(fn: () => Promise<string>) {
    busy = true;
    error = await fn();
    busy = false;
    await load();
  }

  function bind() {
    const pick = open.find((o) => `${o.c.id}/${o.u.id}` === target);
    if (!pick) return;
    void act(() => bindCredential({ vault: pick.c.vaultId ?? '', connection: pick.c.id, user: pick.u.id, identity: key }));
  }
</script>

<div class="section">
  <h4>{$t('sync.bindings.title')}</h4>
  {#if bindings.length === 0}
    <p class="sync-hint">{$t('sync.bindings.none')}</p>
  {:else}
    <ul class="sync-list">
      {#each bindings as b (b.vault + b.connection + b.user)}
        <li>
          <div>{label(b)}</div>
          {#if b.suspendedBy}
            <div class="sync-error">{$t('sync.bindings.suspended', { account: b.suspendedBy.slice(0, 8) })}</div>
            <div class="sync-actions">
              <button type="button" class="primary" on:click={() => act(() => confirmBinding(b))} disabled={busy}>{$t('sync.bindings.confirm')}</button>
            </div>
          {/if}
        </li>
      {/each}
    </ul>
  {/if}
  {#if open.length > 0}
    <div class="sync-actions">
      <select bind:value={target} disabled={busy} aria-label={$t('sync.bindings.connection')}>
        <option value="">—</option>
        {#each open as o (o.c.id + o.u.id)}
          <option value={`${o.c.id}/${o.u.id}`}>{o.c.name} · {o.u.username}</option>
        {/each}
      </select>
      <select bind:value={key} disabled={busy} aria-label={$t('sync.bindings.key')}>
        <option value="">—</option>
        {#each keys as k (k.id)}
          <option value={k.id}>{k.comment || k.fingerprint || k.id}</option>
        {/each}
      </select>
      <button type="button" class="secondary" on:click={bind} disabled={busy || !target || !key}>{$t('sync.bindings.bind')}</button>
    </div>
  {/if}
  <p class="sync-error" role="alert">{error}</p>
</div>
