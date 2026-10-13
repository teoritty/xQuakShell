<script lang="ts">
  // The Sync tab. Not part of the settings draft: every action here is its own operation against
  // the sync account, applied at once, so there is nothing for the dialog's Save to write.
  import { onMount } from 'svelte';
  import SettingsSection from './SettingsSection.svelte';
  import SyncEnableForm from './sync/SyncEnableForm.svelte';
  import SyncAccountPanel from './sync/SyncAccountPanel.svelte';
  import SyncDevicesPanel from './sync/SyncDevicesPanel.svelte';
  import SyncChangesPanel from './sync/SyncChangesPanel.svelte';
  import SyncTrashPanel from './sync/SyncTrashPanel.svelte';
  import { syncStatus } from '../../stores/syncState';
  import { refreshSyncStatus } from '../../actions/syncActions';
  import type { SettingsSearchViewState } from '../settingsSearch';
  import './sync/sync.css';

  export let view: SettingsSearchViewState;

  onMount(refreshSyncStatus);
</script>

<SettingsSection tab="sync" section="account" {view}>
  {#if $syncStatus.enabled && $syncStatus.joinPending}
    <SyncEnableForm initialStep="join" />
  {:else if $syncStatus.enabled}
    <SyncAccountPanel />
  {:else}
    <SyncEnableForm />
  {/if}
</SettingsSection>

{#if $syncStatus.enabled && !$syncStatus.joinPending}
  <SettingsSection tab="sync" section="devices" {view}>
    <SyncDevicesPanel />
  </SettingsSection>
  <SettingsSection tab="sync" section="changes" {view}>
    <SyncChangesPanel />
  </SettingsSection>
  <SettingsSection tab="sync" section="trash" {view}>
    <SyncTrashPanel />
  </SettingsSection>
{/if}
