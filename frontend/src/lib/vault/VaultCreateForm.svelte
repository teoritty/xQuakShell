<script lang="ts">
  import { KeyRound, TriangleAlert } from 'lucide-svelte';
  import { createVault } from '../../actions/vaultActions';
  import {
    evaluatePasswordStrength,
    checkPasswordRequirements,
  } from './passwordStrength';
  import VaultCard from './VaultCard.svelte';
  import PasswordField from './PasswordField.svelte';
  import PasswordStrengthMeter from './PasswordStrengthMeter.svelte';
  import PasswordRequirements from './PasswordRequirements.svelte';

  let password = '';
  let confirmation = '';
  let error = '';
  let loading = false;

  $: strength = evaluatePasswordStrength(password);
  $: checklist = checkPasswordRequirements(password);
  $: mismatch = confirmation.length > 0 && confirmation !== password;
  $: canSubmit = checklist.minLength && password === confirmation && !loading;

  async function handleCreate() {
    if (!canSubmit) return;
    loading = true;
    error = '';
    try {
      await createVault(password);
    } catch (e: any) {
      error = e?.message || 'Could not create the vault';
    } finally {
      loading = false;
    }
  }
</script>

<VaultCard
  wide
  title="Choose a master password"
  subtitle="This password encrypts your connections, keys and secrets. It is the only key to them."
  {error}
>
  <KeyRound slot="icon" size={48} strokeWidth={1.5} />

  <form on:submit|preventDefault={handleCreate}>
    <PasswordField
      bind:value={password}
      ariaLabel="Master password"
      placeholder="Master password"
      disabled={loading}
      autofocus
    />

    {#if password}
      <PasswordStrengthMeter result={strength} />
      <PasswordRequirements {checklist} />
    {/if}

    <PasswordField
      bind:value={confirmation}
      ariaLabel="Confirm master password"
      placeholder="Repeat master password"
      disabled={loading}
    />

    {#if mismatch}
      <p class="mismatch" role="alert">The two passwords do not match.</p>
    {/if}

    <div class="no-recovery">
      <TriangleAlert size={15} strokeWidth={1.8} />
      <p>
        There is no recovery. Nobody, including xQuakShell, can reset this
        password &mdash; forget it and the vault is gone for good.
      </p>
    </div>

    <button type="submit" class="primary" disabled={!canSubmit}>
      {loading ? 'Creating vault...' : 'Create vault'}
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

  .mismatch {
    margin: 0;
    font-size: 11px;
    color: var(--danger);
  }

  .no-recovery {
    display: flex;
    align-items: flex-start;
    gap: 8px;
    padding: 8px 10px;
    border: 1px solid var(--warning);
    border-radius: 4px;
    background: rgba(196, 144, 64, 0.12);
    color: var(--warning);
  }

  /* The card centres its children; this block reads as prose, so opt out. */
  .no-recovery :global(svg) {
    flex-shrink: 0;
    margin-top: 1px;
  }

  .no-recovery p {
    margin: 0;
    font-size: 11px;
    line-height: 1.45;
    text-align: left;
  }
</style>
