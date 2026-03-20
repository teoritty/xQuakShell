<script lang="ts">
  import { unlockVault } from '../stores/api';
  import { Lock } from 'lucide-svelte';

  let masterPassword = '';
  let error = '';
  let loading = false;

  async function handleUnlock() {
    if (!masterPassword) return;
    loading = true;
    error = '';
    try {
      await unlockVault(masterPassword);
    } catch (e: any) {
      error = e?.message || 'Failed to unlock vault';
    } finally {
      loading = false;
    }
  }
</script>

<div class="vault-unlock">
  <div class="vault-card">
    <div class="vault-icon"><Lock size={48} strokeWidth={1.5} /></div>
    <h2>xQuakShell</h2>
    <p class="vault-subtitle">Enter your master password to unlock the vault</p>

    {#if error}
      <div class="vault-error">{error}</div>
    {/if}

    <form on:submit|preventDefault={handleUnlock}>
      <input
        type="password"
        placeholder="Master password..."
        bind:value={masterPassword}
        disabled={loading}
        autofocus
      />
      <button type="submit" disabled={loading || !masterPassword}>
        {loading ? 'Unlocking...' : 'Unlock'}
      </button>
    </form>

    <p class="vault-hint">First time? Enter a new password to create the vault.</p>
  </div>
</div>

<style>
  .vault-unlock {
    display: flex;
    align-items: center;
    justify-content: center;
    flex: 1;
    background: var(--bg-primary);
  }

  .vault-card {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    padding: 40px;
    background: var(--bg-secondary);
    border: 1px solid var(--border-color);
    border-radius: 8px;
    min-width: 340px;
    box-shadow: 0 4px 24px rgba(0,0,0,0.3);
  }

  .vault-icon {
    font-size: 48px;
  }

  h2 {
    font-size: 20px;
    font-weight: 600;
    color: var(--text-bright);
    margin: 0;
  }

  .vault-subtitle {
    font-size: 13px;
    color: var(--text-secondary);
    margin: 0;
  }

  .vault-error {
    padding: 8px 12px;
    background: rgba(211, 47, 47, 0.15);
    border: 1px solid var(--danger);
    border-radius: 4px;
    color: var(--danger);
    font-size: 12px;
    width: 100%;
    text-align: center;
  }

  form {
    display: flex;
    flex-direction: column;
    gap: 8px;
    width: 100%;
  }

  form input {
    width: 100%;
    padding: 8px 12px;
    font-size: 14px;
  }

  form button {
    padding: 8px 16px;
    font-size: 14px;
  }

  .vault-hint {
    font-size: 11px;
    color: var(--text-secondary);
    margin: 0;
    font-style: italic;
  }
</style>
