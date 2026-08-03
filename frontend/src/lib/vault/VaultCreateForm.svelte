<script lang="ts">
  import { KeyRound } from 'lucide-svelte';
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

  // null while empty so the meter renders neutral rather than shouting "Weak"
  // at a field the user has not touched yet.
  $: strength = password ? evaluatePasswordStrength(password) : null;
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

    <!--
      The meter and the checklist are always mounted, and the messages below
      them keep their line whether or not they have anything to say. Revealing
      them per keystroke would shove every field underneath up and down while
      the user is typing into one of them.
    -->
    <PasswordStrengthMeter result={strength} />
    <PasswordRequirements {checklist} />

    <PasswordField
      bind:value={confirmation}
      ariaLabel="Confirm master password"
      placeholder="Repeat master password"
      disabled={loading}
    />

    <p class="mismatch" role="alert">
      {mismatch ? 'The two passwords do not match.' : ''}
    </p>

    <button type="submit" class="primary" disabled={!canSubmit}>
      {loading ? 'Creating vault...' : 'Create vault'}
    </button>

    <p class="no-recovery">
      There is no recovery. Forget this password and the vault is gone for good.
    </p>
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
    /* Holds its line while empty so the button below never jumps. */
    min-height: 15px;
    font-size: 11px;
    color: var(--danger);
  }

  .no-recovery {
    margin: 0;
    font-size: 10px;
    line-height: 1.4;
    color: var(--text-secondary);
    text-align: center;
  }
</style>
