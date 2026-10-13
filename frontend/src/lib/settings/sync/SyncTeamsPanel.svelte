<script lang="ts">
  // Team vaults the account belongs to, and creating one. Choosing a team opens its members and
  // the actions this account's role allows there.
  import { onMount } from 'svelte';
  import SyncTeamDetail from './SyncTeamDetail.svelte';
  import { listTeamsRpc, type Team } from '../../../api/team';
  import { createTeam } from '../../../actions/teamActions';
  import { errorMessage } from '../../../actions/syncActions';
  import { t } from '../../../i18n/messages';

  let teams: Team[] = [];
  let selected: Team | null = null;
  let name = '';
  let busy = false;
  let error = '';

  onMount(load);

  async function load() {
    try {
      teams = await listTeamsRpc();
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
  {#if teams.length === 0}
    <p class="sync-hint">{$t('sync.teams.none')}</p>
  {:else}
    <ul class="sync-list">
      {#each teams as team (team.vault)}
        <li>
          <button type="button" class="ghost" class:active={selected?.vault === team.vault} on:click={() => (selected = team)}>
            {team.name || team.vault.slice(0, 8)}
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
