<script lang="ts">
  import { t } from '../../i18n/messages';
  import { detailsConnection } from '../../stores/appState';
  import { canWrite, requestVaultMove, teams } from '../../stores/teamState';
  import './connectionDetailsShared.css';

  // Where the connection is kept, and the way to keep it somewhere else - one of the connection's
  // settings, so it sits with them rather than in the panel's title bar, where it was easy to miss
  // and too narrow to read a team's name. Shown once there is a choice - a team to share with, or a
  // team it is already in - so without sync nothing changes. Choosing only asks; the move happens in
  // the confirmation dialog, and the field keeps showing the vault the connection is in until then.
  $: conn = $detailsConnection;
  $: current = conn?.vaultId ?? '';
  $: currentTeam = $teams.find((x) => x.vault === current);
  $: locked = current !== '' && !canWrite(currentTeam);
  $: offered = $teams.filter((x) => canWrite(x) || x.vault === current);
  $: show = !!conn && ($teams.length > 0 || current !== '');

  function choose(e: Event & { currentTarget: HTMLSelectElement }) {
    const vault = e.currentTarget.value;
    e.currentTarget.value = current;
    if (conn && vault !== current) requestVaultMove({ connectionIds: [conn.id], vault, folder: '' });
  }
</script>

{#if show}
  <label class="connection-detail-field" title={$t('connection.vault.hint')}>
    <span class="connection-detail-field-label">{$t('connection.vault.label')}</span>
    <select value={current} on:change={choose} disabled={locked}>
      <option value="">{$t('tree.vault.personal')}</option>
      {#each offered as team (team.vault)}
        <option value={team.vault}>{team.name || $t('sync.teams.unnamed')}</option>
      {/each}
      {#if current && !currentTeam}
        <option value={current}>{$t('sync.teams.unnamed')}</option>
      {/if}
    </select>
    {#if locked}
      <span class="view-only-hint">{$t('tree.vault.viewerHint')}</span>
    {/if}
  </label>
{/if}

<style>
  .view-only-hint {
    font-size: 10px;
    color: var(--text-secondary);
  }
</style>
