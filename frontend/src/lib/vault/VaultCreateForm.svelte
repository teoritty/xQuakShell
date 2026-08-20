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
  import { t } from '../../i18n/messages';

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
      error = e?.message || $t('vault.create.failed');
    } finally {
      loading = false;
    }
  }
</script>

<VaultCard
  wide
  title={$t('vault.create.title')}
  subtitle={$t('security.vault.create.subtitle')}
  {error}
>
  <KeyRound slot="icon" size={48} strokeWidth={1.5} />

  <form on:submit|preventDefault={handleCreate}>
    <PasswordField
      bind:value={password}
      ariaLabel={$t('vault.field.master')}
      placeholder={$t('vault.field.master')}
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
      ariaLabel={$t('vault.field.confirm')}
      placeholder={$t('vault.field.repeat')}
      disabled={loading}
    />

    <p class="mismatch" role="alert">
      {mismatch ? $t('vault.create.mismatch') : ''}
    </p>

    <button type="submit" class="primary" disabled={!canSubmit}>
      {loading ? $t('vault.create.busy') : $t('vault.create.submit')}
    </button>

    <p class="no-recovery">
      {$t('security.vault.create.noRecovery')}
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
