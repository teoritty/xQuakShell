<script lang="ts">
  import { KeyRound } from 'lucide-svelte';
  import { completeRecoveryReset } from '../../actions/vaultActions';
  import { evaluatePasswordStrength, checkPasswordRequirements } from './passwordStrength';
  import VaultCard from './VaultCard.svelte';
  import PasswordField from './PasswordField.svelte';
  import PasswordStrengthMeter from './PasswordStrengthMeter.svelte';
  import PasswordRequirements from './PasswordRequirements.svelte';
  import NoRecoveryWarning from './NoRecoveryWarning.svelte';
  import NoRecoveryConsent from './NoRecoveryConsent.svelte';
  import { t } from '../../i18n/messages';

  // Reached only after an unlock that used the recovery key. There is no cancel: the vault is open
  // under a credential the user was told to store away from the machine, and leaving it that way is
  // the state this screen exists to end.
  let password = '';
  let confirmation = '';
  let error = '';
  let loading = false;
  let acknowledged = false;

  $: strength = password ? evaluatePasswordStrength(password) : null;
  $: checklist = checkPasswordRequirements(password);
  $: mismatch = confirmation.length > 0 && confirmation !== password;
  $: canSubmit = checklist.minLength && password === confirmation && acknowledged && !loading;

  async function handleReset() {
    if (!canSubmit) return;
    loading = true;
    error = '';
    try {
      await completeRecoveryReset(password);
    } catch (e: any) {
      error = e?.message || $t('vault.reset.failed');
    } finally {
      loading = false;
    }
  }
</script>

<VaultCard wide title={$t('vault.reset.title')} subtitle={$t('vault.reset.subtitle')} {error}>
  <KeyRound slot="icon" size={48} strokeWidth={1.5} />

  <form on:submit|preventDefault={handleReset}>
    <NoRecoveryWarning />

    <PasswordField
      bind:value={password}
      ariaLabel={$t('security.vault.newPassword')}
      placeholder={$t('security.vault.newPassword')}
      disabled={loading}
      autofocus
    />

    <PasswordStrengthMeter result={strength} />
    <PasswordRequirements {checklist} />

    <div class="field-group">
      <PasswordField
        bind:value={confirmation}
        ariaLabel={$t('vault.field.confirm')}
        placeholder={$t('vault.field.repeat')}
        disabled={loading}
      />
      <p class="mismatch" class:reserved={confirmation.length > 0} role="alert">
        {mismatch ? $t('vault.create.mismatch') : ''}
      </p>
    </div>

    <NoRecoveryConsent bind:acknowledged disabled={loading} />

    <button type="submit" class="primary" disabled={!canSubmit}>
      {loading ? $t('vault.reset.busy') : $t('vault.reset.submit')}
    </button>

    <p class="next-step">
      {$t('security.vault.create.next')}
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

  /* The mismatch message belongs to the field above it, not to the form's own rhythm. Grouping
     them means the reserved line sits four pixels under the input instead of a full form gap away,
     so an empty message no longer reads as a hole between the field and the checkbox. */
  .field-group {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .mismatch {
    margin: 0;
    font-size: 11px;
    line-height: 14px;
    color: var(--danger);
  }

  /* The line is held open only once the confirmation field has something in it, because until then
     a mismatch cannot happen and the space is reserved against nothing. Holding it open regardless
     put a blank row between the last field and the checkbox that read as a layout mistake. The
     single shift this costs lands on the first keystroke into the last field, where the user is
     already looking, rather than on every keystroke. */
  .mismatch.reserved {
    min-height: 14px;
  }

  .next-step {
    margin: 0;
    font-size: 10px;
    line-height: 1.4;
    color: var(--text-secondary);
    text-align: center;
  }
</style>
