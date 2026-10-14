<script lang="ts">
  import { t } from '../../i18n/messages';
  import { createEventDispatcher } from 'svelte';
  import { X } from 'lucide-svelte';
  import type { SaveStatus } from './types';
  import { detailsConnection } from '../../stores/appState';
  import { canWrite, requestVaultMove, teams } from '../../stores/teamState';

  export let saveStatus: SaveStatus = 'idle';

  const dispatch = createEventDispatcher<{ close: void }>();

  // Where the connection is kept, and the way to keep it somewhere else. Shown once there is a
  // choice - a team to share with, or a team it is already in - so without sync nothing changes.
  // Choosing only asks; the move happens in the confirmation dialog, and the field keeps showing
  // the vault the connection is actually in until then.
  $: conn = $detailsConnection;
  $: current = conn?.vaultId ?? '';
  $: currentTeam = $teams.find((x) => x.vault === current);
  $: locked = current !== '' && !canWrite(currentTeam);
  $: offered = $teams.filter((x) => canWrite(x) || x.vault === current);
  $: showVault = !!conn && ($teams.length > 0 || current !== '');

  function choose(e: Event & { currentTarget: HTMLSelectElement }) {
    const vault = e.currentTarget.value;
    e.currentTarget.value = current;
    if (conn && vault !== current) requestVaultMove({ connectionIds: [conn.id], vault, folder: '' });
  }
</script>

<div class="panel-header">
  <div class="panel-header-left">
    <span>{$t('connection.header')}</span>
    <span class="save-indicator">
      {#if saveStatus === 'saving'}Saving...{:else if saveStatus === 'saved'}Saved{/if}
    </span>
  </div>
  {#if showVault}
    <label class="vault-field" title={locked ? $t('tree.vault.viewerHint') : $t('connection.vault.hint')}>
      <span>{$t('connection.vault.label')}</span>
      <select value={current} on:change={choose} disabled={locked}>
        <option value="">{$t('tree.vault.personal')}</option>
        {#each offered as team (team.vault)}
          <option value={team.vault}>{team.name || $t('sync.teams.unnamed')}</option>
        {/each}
        {#if current && !currentTeam}
          <option value={current}>{$t('sync.teams.unnamed')}</option>
        {/if}
      </select>
    </label>
  {/if}
  <button
    type="button"
    class="panel-close-btn"
    title={$t('common.close')}
    on:click={() => dispatch('close')}
  >
    <X size={14} />
  </button>
</div>

<style>
  .panel-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    gap: 8px;
  }

  .panel-header-left {
    display: flex;
    align-items: center;
    gap: 8px;
    min-width: 0;
  }

  .vault-field {
    display: flex;
    align-items: center;
    gap: 4px;
    margin-left: auto;
    min-width: 0;
    font-size: 11px;
    color: var(--text-secondary);
  }

  .vault-field select {
    max-width: 140px;
    font-size: 11px;
  }

  .panel-close-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: none;
    border: none;
    padding: 2px 4px;
    cursor: pointer;
    color: var(--text-secondary);
    border-radius: 2px;
    flex-shrink: 0;
  }

  .panel-close-btn:hover {
    color: var(--danger);
  }

  .save-indicator {
    font-size: 10px;
    color: var(--text-secondary);
    font-style: italic;
  }
</style>
