<script lang="ts">
  import SettingsSection from './SettingsSection.svelte';
  import ConfirmDialog from '../ConfirmDialog.svelte';
  import { enableAuditSecretLogging, disableAuditSecretLogging } from '../../api/audit';
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
    <h4>General</h4>
    <p class="section-desc">Submitted commands are stored locally, on Enter. Disabled by default.</p>
    <label class="checkbox-row">
      <input type="checkbox" bind:checked={draft.auditLogEnabled} />
      Enable audit log
    </label>
  </div>
</SettingsSection>

<SettingsSection tab="audit" section="retention" {view}>
  <div class="section">
    <h4>Retention</h4>
    <p class="section-desc">Old entries are deleted automatically and cannot be recovered.</p>
    <label class="checkbox-row">
      <input type="radio" bind:group={draft.auditRetentionMode} value="days" disabled={!draft.auditLogEnabled} />
      By time
    </label>
    <label class="setting-row setting-sub">
      <span>Keep entries for (days)</span>
      <input type="number" bind:value={draft.auditRetentionDays} min="1" max="365" disabled={!draft.auditLogEnabled || draft.auditRetentionMode !== 'days'} />
    </label>
    <label class="checkbox-row">
      <input type="radio" bind:group={draft.auditRetentionMode} value="count" disabled={!draft.auditLogEnabled} />
      By count
    </label>
    <label class="setting-row setting-sub">
      <span>Maximum entries</span>
      <input type="number" bind:value={draft.auditRetentionCount} min="10" max="10000" disabled={!draft.auditLogEnabled || draft.auditRetentionMode !== 'count'} />
    </label>
  </div>
</SettingsSection>

<SettingsSection tab="audit" section="privacy" {view}>
  <div class="section">
    <h4>Privacy</h4>
    <label class="checkbox-row">
      <input type="checkbox" bind:checked={draft.auditShowUsername} disabled={!draft.auditLogEnabled} />
      Log &amp; show username
    </label>
    <label class="checkbox-row">
      <input type="checkbox" bind:checked={draft.auditShowConnection} disabled={!draft.auditLogEnabled} />
      Log &amp; show connection (name and host)
    </label>
  </div>
</SettingsSection>

<SettingsSection tab="audit" section="secrets" {view}>
  <div class="section">
    <h4>Sensitive data</h4>
    <p class="section-desc">Passwords and secrets are logged in plaintext until you lock the vault or restart the app. Never saved to the vault.</p>
    <label class="checkbox-row">
      <input type="checkbox" checked={auditLogSecrets} on:change={handleToggle} disabled={!draft.auditLogEnabled} />
      Log secrets this session
    </label>
  </div>
</SettingsSection>

<ConfirmDialog
  show={confirmShow}
  title="Enable secret logging"
  message="Secrets will be stored in plaintext in the local audit database. This applies only until you lock the vault or restart the app."
  critical={true}
  requireCheckbox={true}
  checkboxLabel="I understand that sensitive data will be logged in plaintext"
  confirmLabel="Enable"
  cancelLabel="Cancel"
  on:confirm={confirmSecrets}
  on:cancel={cancelSecrets}
/>
