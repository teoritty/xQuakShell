<script lang="ts">
  // The Sync tab: everything on one page, nothing behind steps. Turned off, it is the one form that
  // turns sync on. Turned on, it shows the account and only what needs the person - a stopped
  // account, a new device, a change waiting, a conflict - then teams and the trash. Fingerprints,
  // the sync password, the device list, Shred now and turning sync off sit behind "Advanced", which
  // a search opens by itself so a setting someone looks for by name is never hidden from them.
  //
  // Not part of the settings draft: every action here is its own operation against the sync
  // account, applied at once, so there is nothing for the dialog's Save to write.
  import { onMount } from 'svelte';
  import SettingsSection from './SettingsSection.svelte';
  import SyncConnectForm from './sync/SyncConnectForm.svelte';
  import SyncOverview from './sync/SyncOverview.svelte';
  import SyncAdvancedPanel from './sync/SyncAdvancedPanel.svelte';
  import SyncDevicesPanel from './sync/SyncDevicesPanel.svelte';
  import SyncChangesPanel from './sync/SyncChangesPanel.svelte';
  import SyncTrashPanel from './sync/SyncTrashPanel.svelte';
  import SyncTeamsPanel from './sync/SyncTeamsPanel.svelte';
  import SyncBindingsPanel from './sync/SyncBindingsPanel.svelte';
  import SyncPluginsPanel from './sync/SyncPluginsPanel.svelte';
  import { syncStatus } from '../../stores/syncState';
  import { refreshSyncStatus } from '../../actions/syncActions';
  import type { SettingsSearchViewState } from '../settingsSearch';
  import { t } from '../../i18n/messages';
  import './sync/sync.css';

  export let view: SettingsSearchViewState;

  // A display preference, so it lives in localStorage - never anything about the account itself.
  const ADVANCED_KEY = 'xqs.sync.advanced';
  let advanced = readAdvanced();

  function readAdvanced(): boolean {
    try {
      return localStorage.getItem(ADVANCED_KEY) === '1';
    } catch {
      return false;
    }
  }

  function setAdvanced(on: boolean) {
    advanced = on;
    try {
      localStorage.setItem(ADVANCED_KEY, on ? '1' : '0');
    } catch {
      // Storage may be unavailable; the switch still works for this session.
    }
  }

  $: showAdvanced = advanced || view.isSearching;
  $: enabled = $syncStatus.enabled;
  $: ready = enabled && !$syncStatus.joinPending;

  onMount(refreshSyncStatus);
</script>

<SettingsSection tab="sync" section="account" {view}>
  {#if enabled}
    <SyncOverview />
  {:else}
    <SyncConnectForm advanced={showAdvanced} />
  {/if}
</SettingsSection>

{#if ready}
  <SettingsSection tab="sync" section="changes" {view}>
    <SyncChangesPanel />
  </SettingsSection>
  <SettingsSection tab="sync" section="plugins" {view}>
    <SyncPluginsPanel advanced={showAdvanced} />
  </SettingsSection>
  <SettingsSection tab="sync" section="teams" {view}>
    <SyncTeamsPanel />
  </SettingsSection>
  <SettingsSection tab="sync" section="credentials" {view}>
    <SyncBindingsPanel />
  </SettingsSection>
  <SettingsSection tab="sync" section="trash" {view}>
    <SyncTrashPanel advanced={showAdvanced} />
  </SettingsSection>
{/if}

{#if view.activeTab === 'sync' && !view.isSearching}
  <label class="checkbox-row sync-advanced-toggle">
    <input type="checkbox" checked={advanced} on:change={(e) => setAdvanced(e.currentTarget.checked)} />
    {$t('sync.advanced.toggle')}
  </label>
{/if}

{#if ready && showAdvanced}
  <SettingsSection tab="sync" section="devices" {view}>
    <SyncDevicesPanel />
  </SettingsSection>
  <SettingsSection tab="sync" section="advanced" {view}>
    <SyncAdvancedPanel />
  </SettingsSection>
{/if}
