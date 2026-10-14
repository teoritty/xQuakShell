<script lang="ts">
  // The one place a move between vaults is confirmed, whichever way it was asked for - a drop on a
  // team, the tree's menu, the editor's "Stored in" field. One dialog, so the person reads the same
  // account of who gains or loses the connection every time.
  import ConfirmDialog from '../ConfirmDialog.svelte';
  import { connections, sessions, showError } from '../../stores/appState';
  import { pendingVaultMove, teams } from '../../stores/teamState';
  import { moveConnectionsToVault } from '../../actions/teamActions';
  import { describeVaultMove } from './vaultMovePrompt';
  import { t } from '../../i18n/messages';

  let busy = false;
  $: req = $pendingVaultMove;
  $: prompt = req ? describeVaultMove(req, $connections, $teams, $sessions, $t) : null;

  async function confirm() {
    if (!req || prompt?.blocked || busy) return;
    const { connectionIds, vault, folder } = req;
    busy = true;
    const error = await moveConnectionsToVault(connectionIds, vault, folder);
    busy = false;
    pendingVaultMove.set(null);
    if (error) showError(error);
  }
</script>

<ConfirmDialog
  show={!!prompt}
  title={prompt?.title ?? ''}
  message={prompt?.message ?? ''}
  confirmLabel={$t('security.tree.vaultMove.confirm')}
  confirmDisabled={busy || !!prompt?.blocked}
  on:confirm={confirm}
  on:cancel={() => !busy && pendingVaultMove.set(null)}
>
  <svelte:fragment slot="body">
    {#if prompt?.blocked}<p class="vault-move-blocked" role="alert">{prompt.blocked}</p>{/if}
  </svelte:fragment>
</ConfirmDialog>

<style>
  .vault-move-blocked {
    margin: 8px 0 0;
    color: var(--danger);
    font-size: 12px;
  }
</style>
