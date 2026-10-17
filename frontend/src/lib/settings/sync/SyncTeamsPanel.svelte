<script lang="ts">
  // Team vaults the account belongs to, and creating one. Choosing a team opens its members and
  // the actions this account's role allows there. The account's own fingerprint is shown here
  // because this is where it is needed: an admin compares it, over another channel, before
  // inviting this account, and a person who cannot read theirs out can only tick "matches" blind.
  // The contact code carries it in a form the admin pastes rather than reads, so the application
  // makes the comparison.
  import SyncTeamDetail from './SyncTeamDetail.svelte';
  import { listTeamsRpc, type Team } from '../../../api/team';
  import { createTeam } from '../../../actions/teamActions';
  import { errorMessage } from '../../../actions/syncActions';
  import { syncRevision, syncStatus } from '../../../stores/syncState';
  import { teams as sharedTeams } from '../../../stores/teamState';
  import { formatContactCode } from './contactCode';
  import { t } from '../../../i18n/messages';

  let teams: Team[] = [];
  let selected: Team | null = null;
  let name = '';
  let busy = false;
  let error = '';
  let copied = false;

  $: void load($syncRevision);
  $: contactCode = formatContactCode($syncStatus.username ?? '',$syncStatus.accountFingerprint ?? '');

  async function copyCode() {
    try {
      await navigator.clipboard.writeText(contactCode);
      copied = true;
      setTimeout(() => (copied = false), 2000);
    } catch (e) {
      error = errorMessage(e);
    }
  }

  async function load(..._deps: unknown[]) {
    try {
      teams = await listTeamsRpc();
      // The tree and the editor place items by the same list; this read keeps theirs current too.
      sharedTeams.set(teams);
      error = '';
      if (selected) selected = teams.find((x) => x.vault === selected?.vault) ?? null;
    } catch (e) {
      error = errorMessage(e);
    }
  }

  async function create() {
    busy = true;
    const res = await createTeam(name);
    busy = false;
    error = res.error;
    if (!res.error) {
      name = '';
      await load();
    }
  }
</script>

<div class="section">
  <h4>{$t('sync.teams.title')}</h4>
  {#if $syncStatus.accountFingerprint}
    <p class="sync-hint">{$t('security.sync.teams.ownHint')}</p>
    <p class="sync-fingerprint">{$syncStatus.accountFingerprint}</p>
    {#if contactCode}
      <div class="sync-actions">
        <button type="button" class="secondary" on:click={copyCode}>{$t('sync.teams.code.copy')}</button>
        {#if copied}<span class="sync-meta" role="status">{$t('sync.teams.code.copied')}</span>{/if}
      </div>
    {/if}
  {/if}
  {#if teams.length === 0}
    <p class="sync-hint">{$t('sync.teams.none')}</p>
  {:else}
    <ul class="sync-list">
      {#each teams as team (team.vault)}
        <li>
          <button type="button" class="ghost" class:active={selected?.vault === team.vault} on:click={() => (selected = team)}>
            {team.name || $t('sync.teams.unnamed')}
          </button>
          <span class="sync-meta">{$t('sync.teams.role.' + team.role)}</span>
        </li>
      {/each}
    </ul>
  {/if}
  <div class="sync-actions">
    <input type="text" bind:value={name} placeholder={$t('sync.teams.name')} aria-label={$t('sync.teams.name')} disabled={busy} />
    <button type="button" class="primary" on:click={create} disabled={busy || !name.trim()}>{$t('sync.teams.create')}</button>
  </div>
  <p class="sync-error" role="alert">{error}</p>
</div>

{#if selected}
  <SyncTeamDetail team={selected} on:changed={load} />
{/if}
