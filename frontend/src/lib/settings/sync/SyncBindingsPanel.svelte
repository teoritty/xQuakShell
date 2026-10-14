<script lang="ts">
  // This member's own credentials for team connections whose users take one from each member.
  // A binding another member suspended by repointing the connection shows who did it and waits for
  // a confirmation of the new address before the credential is used again.
  import { listCredentialBindingsRpc, listTeamMembersRpc, type CredentialBinding } from '../../../api/team';
  import { bindCredential, confirmBinding } from '../../../actions/teamActions';
  import { errorMessage } from '../../../actions/syncActions';
  import { fetchKeys, type StoredKey } from '../../../api/keys';
  import { connections } from '../../../stores/appState';
  import { syncRevision } from '../../../stores/syncState';
  import { t } from '../../../i18n/messages';

  let bindings: CredentialBinding[] = [];
  let keys: StoredKey[] = [];
  // Who suspended a binding, by name: the account id alone told the person nothing.
  let names: Record<string, string> = {};
  let target = '';
  let key = '';
  let busy = false;
  let error = '';

  // Every user of a team connection that takes its credential from each member.
  $: open = $connections.flatMap((c) =>
    c.vaultId ? (c.users ?? []).filter((u) => u.credentialSource === 'personal').map((u) => ({ c, u })) : [],
  );

  $: void load($syncRevision);

  async function load(..._deps: unknown[]) {
    try {
      [bindings, keys] = await Promise.all([listCredentialBindingsRpc(), fetchKeys()]);
      names = await suspenderNames(bindings);
      error = '';
    } catch (e) {
      error = errorMessage(e);
    }
  }

  async function suspenderNames(list: CredentialBinding[]): Promise<Record<string, string>> {
    const out: Record<string, string> = {};
    for (const vault of new Set(list.filter((b) => b.suspendedBy).map((b) => b.vault))) {
      for (const m of await listTeamMembersRpc(vault)) if (m.username) out[m.account] = m.username;
    }
    return out;
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

{#if bindings.length > 0 || open.length > 0}
<div class="section">
  <h4>{$t('sync.bindings.title')}</h4>
  {#if bindings.length > 0}
    <ul class="sync-list">
      {#each bindings as b (b.vault + b.connection + b.user)}
        <li>
          <div>{label(b)}</div>
          {#if b.suspendedBy}
            <div class="sync-error">{$t('sync.bindings.suspended', { account: names[b.suspendedBy] ?? $t('sync.teams.unknownMember') })}</div>
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
{/if}
