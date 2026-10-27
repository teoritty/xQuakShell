<script lang="ts">
  // Teams this account was added to and has not joined. Anyone on the server can add anyone to a
  // team, so being added is only an invitation: nothing from the team reaches the connection tree
  // until the person accepts it here, after comparing the inviter's fingerprint over another
  // channel. Declining leaves the team.
  import { createEventDispatcher } from 'svelte';
  import { listTeamInvitationsRpc, type TeamInvitation } from '../../../api/team';
  import { acceptInvitation, declineInvitation } from '../../../actions/teamActions';
  import { errorMessage } from '../../../actions/syncActions';
  import { syncRevision } from '../../../stores/syncState';
  import { t } from '../../../i18n/messages';

  const dispatch = createEventDispatcher();

  let invitations: TeamInvitation[] = [];
  let busy = false;
  let declining = '';
  let error = '';

  $: void load($syncRevision);

  async function load(..._deps: unknown[]) {
    try {
      invitations = await listTeamInvitationsRpc();
      error = '';
    } catch (e) {
      error = errorMessage(e);
    }
  }

  async function answer(fn: () => Promise<string>) {
    busy = true;
    error = await fn();
    busy = false;
    await load();
    dispatch('changed');
  }

  function accept(vault: string) {
    declining = '';
    void answer(() => acceptInvitation(vault));
  }

  function declineClicked(vault: string) {
    if (declining !== vault) {
      declining = vault;
      return;
    }
    declining = '';
    void answer(() => declineInvitation(vault));
  }

  function inviterLabel(inv: TeamInvitation): string {
    return inv.inviterUsername || $t('sync.teams.invitations.unknownInviter');
  }
</script>

{#if invitations.length > 0 || error}
  <div class="section">
    <h4>{$t('sync.teams.invitations.title')}</h4>
    <p class="sync-hint">{$t('sync.teams.invitations.hint')}</p>
    <ul class="sync-list">
      {#each invitations as inv (inv.vault)}
        <li>
          <span>
            {$t('sync.teams.invitations.from', {
              inviter: inviterLabel(inv),
              team: inv.name || $t('sync.teams.invitations.unnamed'),
              role: $t('sync.teams.role.' + inv.role),
            })}
          </span>
          <span class="sync-fingerprint">{inv.inviterFingerprint}</span>
          <span class="sync-meta">
            {#if inv.inviterVerified}
              {$t('sync.teams.verified')}
            {:else if inv.inviterKnown}
              {$t('sync.teams.invitations.known')}
            {:else}
              {$t('sync.teams.unverified')}
            {/if}
          </span>
          <div class="sync-actions">
            <button type="button" class="primary" on:click={() => accept(inv.vault)} disabled={busy}>
              {$t('sync.teams.invitations.accept')}
            </button>
            <button type="button" class="secondary" on:click={() => declineClicked(inv.vault)} disabled={busy}>
              {declining === inv.vault ? $t('sync.teams.invitations.declineConfirm') : $t('sync.teams.invitations.decline')}
            </button>
          </div>
        </li>
      {/each}
    </ul>
    <p class="sync-error" role="alert">{error}</p>
  </div>
{/if}
