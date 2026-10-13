<script lang="ts">
  // One team: its members, inviting with a fingerprint comparison, roles, removal, leaving, and
  // moving a personal connection in. The fingerprint shown at the invite is the one sent back, so
  // the invite fails if the account's key changed after the person compared it.
  import { createEventDispatcher } from 'svelte';
  import { listTeamMembersRpc, type Team, type TeamInvitee, type TeamMember } from '../../../api/team';
  import {
    changeRole,
    inviteMember,
    leaveTeam,
    lookupInvitee,
    moveConnectionToTeam,
    removeMember,
    verifyMember,
  } from '../../../actions/teamActions';
  import { errorMessage } from '../../../actions/syncActions';
  import { connections } from '../../../stores/appState';
  import { t } from '../../../i18n/messages';

  export let team: Team;

  const dispatch = createEventDispatcher();
  let members: TeamMember[] = [];
  let username = '';
  let invitee: TeamInvitee | null = null;
  let compared = false;
  let role = 'editor';
  let moving = '';
  let busy = false;
  let error = '';

  $: manager = team.role === 'owner' || team.role === 'admin';
  $: writable = manager || team.role === 'editor';
  $: personal = $connections.filter((c) => !c.vaultId);
  // Reloads whenever another team is chosen.
  $: if (team) void load();

  async function load() {
    try {
      members = await listTeamMembersRpc(team.vault);
      error = '';
    } catch (e) {
      error = errorMessage(e);
    }
  }

  async function act(fn: () => Promise<string>) {
    busy = true;
    error = await fn();
    busy = false;
    await load();
    dispatch('changed');
  }

  async function lookup() {
    busy = true;
    const res = await lookupInvitee(username);
    busy = false;
    invitee = res.invitee ?? null;
    compared = invitee?.verified ?? false;
    error = res.error ?? '';
  }

  async function invite() {
    if (!invitee) return;
    const current = invitee;
    await act(() => inviteMember(team.vault, current, role, compared));
    if (!error) {
      invitee = null;
      username = '';
    }
  }
</script>

<div class="section">
  <h4>{team.name}</h4>
  <ul class="sync-list">
    {#each members as m (m.account)}
      <li>
        <div>{m.username || m.account.slice(0, 8)} {#if m.current}({$t('sync.devices.this')}){/if}</div>
        <div class="sync-meta">{m.fingerprint}</div>
        <div class="sync-meta">{m.verified ? $t('sync.teams.verified') : $t('sync.teams.unverified')}</div>
        <div class="sync-actions">
          {#if manager && m.role !== 'owner' && !m.current}
            <select value={m.role} on:change={(e) => act(() => changeRole(team.vault, m.account, e.currentTarget.value))} disabled={busy}>
              <option value="admin">{$t('sync.teams.role.admin')}</option>
              <option value="editor">{$t('sync.teams.role.editor')}</option>
              <option value="viewer">{$t('sync.teams.role.viewer')}</option>
            </select>
            <button type="button" class="secondary" on:click={() => act(() => removeMember(team.vault, m.account))} disabled={busy}>
              {$t('sync.teams.remove')}
            </button>
          {:else}
            <span class="sync-meta">{$t('sync.teams.role.' + m.role)}</span>
          {/if}
          {#if !m.verified && !m.current}
            <button type="button" class="secondary" on:click={() => act(() => verifyMember(team.vault, m.account, m.fingerprint))} disabled={busy}>
              {$t('sync.teams.markVerified')}
            </button>
          {/if}
        </div>
      </li>
    {/each}
  </ul>

  {#if manager}
    <h4>{$t('sync.teams.invite')}</h4>
    <div class="sync-actions">
      <input type="text" bind:value={username} placeholder={$t('sync.enable.username')} aria-label={$t('sync.enable.username')} disabled={busy} />
      <button type="button" class="secondary" on:click={lookup} disabled={busy || !username.trim()}>{$t('sync.teams.lookup')}</button>
    </div>
    {#if invitee}
      <p class="sync-hint">{$t('sync.teams.compare')}</p>
      <p class="sync-fingerprint">{invitee.fingerprint}</p>
      <label><input type="checkbox" bind:checked={compared} /> {$t('sync.teams.compared')}</label>
      <div class="sync-actions">
        <select bind:value={role} disabled={busy}>
          <option value="admin">{$t('sync.teams.role.admin')}</option>
          <option value="editor">{$t('sync.teams.role.editor')}</option>
          <option value="viewer">{$t('sync.teams.role.viewer')}</option>
        </select>
        <button type="button" class="primary" on:click={invite} disabled={busy}>{$t('sync.teams.invite')}</button>
      </div>
    {/if}
  {/if}

  {#if writable && personal.length > 0}
    <h4>{$t('sync.teams.move')}</h4>
    <div class="sync-actions">
      <select bind:value={moving} disabled={busy} aria-label={$t('sync.teams.move')}>
        <option value="">—</option>
        {#each personal as c (c.id)}
          <option value={c.id}>{c.name}</option>
        {/each}
      </select>
      <button type="button" class="secondary" on:click={() => act(() => moveConnectionToTeam(moving, team.vault))} disabled={busy || !moving}>
        {$t('sync.teams.moveAction')}
      </button>
    </div>
    <p class="sync-hint">{$t('sync.teams.moveHint')}</p>
  {/if}

  {#if team.role !== 'owner'}
    <div class="sync-actions">
      <button type="button" class="danger" on:click={() => act(() => leaveTeam(team.vault))} disabled={busy}>{$t('sync.teams.leave')}</button>
    </div>
  {/if}
  <p class="sync-error" role="alert">{error}</p>
</div>
