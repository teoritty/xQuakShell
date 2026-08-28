<script lang="ts">
  import { Lock } from 'lucide-svelte';
  import { unlockVault, warmupAfterVaultOpened } from '../../actions/vaultActions';
  import { vaultExists } from '../../stores/appState';
  import VaultCard from './VaultCard.svelte';
  import PasswordField from './PasswordField.svelte';
  import MigrateKeysWizard from '../keys/MigrateKeysWizard.svelte';
  import { completeKeyMigration, planKeyMigration, type PendingKey } from '../../api/keys';
  import { t } from '../../i18n/messages';

  let masterPassword = '';
  let error = '';
  let loading = false;
  let showMigration = false;
  let pendingKeys: PendingKey[] = [];
  let migrationError = '';
  let migrating = false;

  async function handleUnlock() {
    if (!masterPassword || loading) return;
    loading = true;
    error = '';
    try {
      await unlockVault(masterPassword);
    } catch (e: any) {
      const message = e?.message || $t('vault.unlock.failed');
      // The vault file went missing while the app was running (moved, deleted,
      // or a portable drive unplugged). Send the user to the create screen
      // rather than leaving them retyping a password against nothing.
      if (message.includes('vault not found')) {
        vaultExists.set(false);
        return;
      }
      // A vault one schema behind fails to unlock for a reason that is not a wrong password, and
      // the two need opposite screens. Asking the backend for a plan answers that structurally
      // instead of matching the message text: a plan comes back only when the password was right
      // and an upgrade is genuinely due.
      if (await offerMigration()) return;
      error = message;
    } finally {
      loading = false;
    }
  }

  async function offerMigration(): Promise<boolean> {
    try {
      const plan = await planKeyMigration(masterPassword);
      if (!plan.required) return false;
      pendingKeys = plan.keys;
      migrationError = '';
      showMigration = true;
      return true;
    } catch {
      return false;
    }
  }

  async function runMigration(event: CustomEvent) {
    migrating = true;
    migrationError = '';
    try {
      const report = await completeKeyMigration(masterPassword, event.detail);
      showMigration = false;
      if (report && report.skipped.length > 0) {
        error = $t('vault.migration.partial', {
          count: report.skipped.length,
          backupPath: report.backupPath,
        });
      }
      await warmupAfterVaultOpened();
    } catch (e: any) {
      migrationError = e?.message || $t('vault.migration.failed');
    } finally {
      migrating = false;
    }
  }
</script>

<VaultCard title="xQuakShell" subtitle={$t('vault.unlock.subtitle')} {error}>
  <Lock slot="icon" size={48} strokeWidth={1.5} />

  <form on:submit|preventDefault={handleUnlock}>
    <PasswordField
      bind:value={masterPassword}
      ariaLabel={$t('vault.field.master')}
      placeholder={$t('vault.field.master')}
      disabled={loading}
      autofocus
    />
    <button type="submit" class="primary" disabled={loading || !masterPassword}>
      {loading ? $t('vault.unlock.busy') : $t('vault.unlock.submit')}
    </button>
  </form>
</VaultCard>

<style>
  form {
    display: flex;
    flex-direction: column;
    gap: 10px;
    width: 100%;
  }

  form button {
    padding: 8px 16px;
    font-size: 14px;
  }
</style>

<MigrateKeysWizard
  bind:show={showMigration}
  pending={pendingKeys}
  busy={migrating}
  error={migrationError}
  on:submit={runMigration}
  on:close={() => (showMigration = false)}
/>
