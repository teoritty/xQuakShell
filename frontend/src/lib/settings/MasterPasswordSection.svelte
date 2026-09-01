<script lang="ts">
  import { onMount } from 'svelte';
  import { changeMasterPassword, regenerateRecoveryKey } from '../../actions/vaultActions';
  import { hasRecoveryKeyRpc } from '../../api/vault';
  import { checkPasswordRequirements } from '../vault/passwordStrength';
  import PasswordField from '../vault/PasswordField.svelte';
  import { t } from '../../i18n/messages';

  // Both actions re-authenticate even though the vault is already open. This screen is reachable on
  // an unattended unlocked machine, and between them they can print a credential that opens
  // everything and lock the real owner out.
  let currentPassword = '';
  let newPassword = '';
  let busy: '' | 'change' | 'regenerate' = '';
  let error = '';
  let hasKey = false;

  $: checklist = checkPasswordRequirements(newPassword);
  $: canChange = !!currentPassword && checklist.minLength && !busy;

  onMount(() => {
    void refreshHasKey();
  });

  async function refreshHasKey() {
    try {
      hasKey = await hasRecoveryKeyRpc();
    } catch {
      // Only decides which sentence is shown. A failure here must not block the actions below,
      // which are exactly what someone with a broken vault state needs to reach.
      hasKey = false;
    }
  }

  async function change() {
    if (!canChange) return;
    busy = 'change';
    error = '';
    try {
      await changeMasterPassword(currentPassword, newPassword);
      currentPassword = '';
      newPassword = '';
      await refreshHasKey();
    } catch (e: any) {
      error = e?.message || $t('vault.reset.failed');
    } finally {
      busy = '';
    }
  }

  async function regenerate() {
    if (!currentPassword || busy) return;
    busy = 'regenerate';
    error = '';
    try {
      await regenerateRecoveryKey(currentPassword);
      currentPassword = '';
      await refreshHasKey();
    } catch (e: any) {
      error = e?.message || $t('vault.unlock.failed');
    } finally {
      busy = '';
    }
  }
</script>

<div class="section">
  <h4>{$t('security.vault.section')}</h4>

  <p class="state">{hasKey ? $t('security.vault.hasKey') : $t('security.vault.noKey')}</p>

  <PasswordField
    bind:value={currentPassword}
    ariaLabel={$t('security.vault.currentPassword')}
    placeholder={$t('security.vault.currentPassword')}
    disabled={!!busy}
    compact
  />

  <PasswordField
    bind:value={newPassword}
    ariaLabel={$t('security.vault.newPassword')}
    placeholder={$t('security.vault.newPassword')}
    disabled={!!busy}
    compact
  />

  <p class="error" role="alert">{error}</p>

  <div class="actions">
    <button type="button" class="primary" on:click={change} disabled={!canChange}>
      {busy === 'change' ? $t('security.vault.changeBusy') : $t('security.vault.change')}
    </button>
    <button type="button" class="secondary" on:click={regenerate} disabled={!currentPassword || !!busy}>
      {busy === 'regenerate' ? $t('security.vault.regenerateBusy') : $t('security.vault.regenerate')}
    </button>
  </div>

  <p class="hint">{$t('security.vault.changeHint')}</p>
  <p class="hint">{$t('security.vault.regenerateHint')}</p>
</div>

<style>
  .state {
    margin: 0;
    font-size: 12px;
    color: var(--text-secondary);
  }

  /* A password is not a long value and the settings pane is wide. Left to fill it, the two fields
     read as the subject of the screen rather than as two rows among the security settings. */
  .section :global(.password-field) {
    max-width: 280px;
  }

  .error {
    margin: 0;
    /* Holds its line while empty so the buttons never jump when a password is refused. */
    min-height: 15px;
    font-size: 11px;
    color: var(--danger);
  }

  .actions {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }

  .hint {
    margin: 0;
    font-size: 11px;
    line-height: 1.4;
    color: var(--text-secondary);
  }
</style>
