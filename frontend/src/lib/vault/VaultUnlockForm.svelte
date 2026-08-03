<script lang="ts">
  import { Lock } from 'lucide-svelte';
  import { unlockVault } from '../../actions/vaultActions';
  import { vaultExists } from '../../stores/appState';
  import VaultCard from './VaultCard.svelte';
  import PasswordField from './PasswordField.svelte';

  let masterPassword = '';
  let error = '';
  let loading = false;

  async function handleUnlock() {
    if (!masterPassword || loading) return;
    loading = true;
    error = '';
    try {
      await unlockVault(masterPassword);
    } catch (e: any) {
      const message = e?.message || 'Could not unlock the vault';
      // The vault file went missing while the app was running (moved, deleted,
      // or a portable drive unplugged). Send the user to the create screen
      // rather than leaving them retyping a password against nothing.
      if (message.includes('vault not found')) {
        vaultExists.set(false);
        return;
      }
      error = message;
    } finally {
      loading = false;
    }
  }
</script>

<VaultCard title="xQuakShell" subtitle="Enter your master password to unlock the vault" {error}>
  <Lock slot="icon" size={48} strokeWidth={1.5} />

  <form on:submit|preventDefault={handleUnlock}>
    <PasswordField
      bind:value={masterPassword}
      ariaLabel="Master password"
      placeholder="Master password"
      disabled={loading}
      autofocus
    />
    <button type="submit" class="primary" disabled={loading || !masterPassword}>
      {loading ? 'Unlocking...' : 'Unlock'}
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
