<script lang="ts">
  import SettingsSection from './SettingsSection.svelte';
  import ConfirmDialog from '../ConfirmDialog.svelte';
  import { enableAuditSecretLogging, disableAuditSecretLogging } from '../../api/audit';
  import { t } from '../../i18n/messages';
  import type { SettingsSearchViewState } from '../settingsSearch';
  import type { SettingsDraft } from './settingsDraft';

  export let view: SettingsSearchViewState;
  // Owns every auditLog* field of the draft. Secret logging is not one of them: it is session
  // state the backend turns on directly and forgets on lock, never part of the saved payload.
  export let draft: SettingsDraft;
  export let auditLogSecrets: boolean;

  let confirmShow = false;

  // Turning secret logging ON is refused until the confirmation comes back, so the checkbox is
  // forced back off first: leaving it visually checked while the dialog is open would report a
  // state the backend is not in, and a user who cancels would believe secrets are being logged.
  function handleToggle(e: Event) {
    const input = e.target as HTMLInputElement;
    if (!input.checked) {
      auditLogSecrets = false;
      disableAuditSecretLogging();
      return;
    }
    input.checked = false;
    confirmShow = true;
  }

  async function confirmSecrets() {
    confirmShow = false;
    const ok = await enableAuditSecretLogging(true);
    if (ok) auditLogSecrets = true;
  }

  function cancelSecrets() {
    confirmShow = false;
    auditLogSecrets = false;
  }
</script>

<SettingsSection tab="audit" section="general" {view}>
  <div class="section">
    <h4>{$t('settings.audit.general.title')}</h4>
    <p class="section-desc">{$t('settings.audit.general.desc')}</p>
    <label class="checkbox-row">
      <input type="checkbox" bind:checked={draft.auditLogEnabled} />
      {$t('settings.audit.general.enable')}
    </label>
  </div>
</SettingsSection>

<SettingsSection tab="audit" section="retention" {view}>
  <div class="section">
    <h4>{$t('settings.audit.retention.title')}</h4>
    <p class="section-desc">{$t('settings.audit.retention.desc')}</p>
    <label class="checkbox-row">
      <input type="radio" bind:group={draft.auditRetentionMode} value="days" disabled={!draft.auditLogEnabled} />
      {$t('settings.audit.retention.byTime')}
    </label>
    <label class="setting-row setting-sub">
      <span>{$t('settings.audit.retention.days')}</span>
      <input type="number" bind:value={draft.auditRetentionDays} min="1" max="365" disabled={!draft.auditLogEnabled || draft.auditRetentionMode !== 'days'} />
    </label>
    <label class="checkbox-row">
      <input type="radio" bind:group={draft.auditRetentionMode} value="count" disabled={!draft.auditLogEnabled} />
      {$t('settings.audit.retention.byCount')}
    </label>
    <label class="setting-row setting-sub">
      <span>{$t('settings.audit.retention.max')}</span>
      <input type="number" bind:value={draft.auditRetentionCount} min="10" max="10000" disabled={!draft.auditLogEnabled || draft.auditRetentionMode !== 'count'} />
    </label>
  </div>
</SettingsSection>

<SettingsSection tab="audit" section="privacy" {view}>
  <div class="section">
    <h4>{$t('settings.audit.privacy.title')}</h4>
    <label class="checkbox-row">
      <input type="checkbox" bind:checked={draft.auditShowUsername} disabled={!draft.auditLogEnabled} />
      {$t('settings.audit.privacy.username')}
    </label>
    <label class="checkbox-row">
      <input type="checkbox" bind:checked={draft.auditShowConnection} disabled={!draft.auditLogEnabled} />
      {$t('settings.audit.privacy.connection')}
    </label>
  </div>
</SettingsSection>

<SettingsSection tab="audit" section="secrets" {view}>
  <div class="section">
    <h4>{$t('settings.audit.secrets.title')}</h4>
    <p class="section-desc">{$t('security.audit.secrets.desc')}</p>
    <label class="checkbox-row">
      <input type="checkbox" checked={auditLogSecrets} on:change={handleToggle} disabled={!draft.auditLogEnabled} />
      {$t('settings.audit.secrets.enable')}
    </label>
  </div>
</SettingsSection>

<ConfirmDialog
  show={confirmShow}
  title={$t('security.audit.secrets.confirm.title')}
  message={$t('security.audit.secrets.confirm.message')}
  critical={true}
  requireCheckbox={true}
  checkboxLabel={$t('security.audit.secrets.confirm.checkbox')}
  confirmLabel={$t('security.audit.secrets.confirm.accept')}
  cancelLabel={$t('security.audit.secrets.confirm.cancel')}
  on:confirm={confirmSecrets}
  on:cancel={cancelSecrets}
/>
