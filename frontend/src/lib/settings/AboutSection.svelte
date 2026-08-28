<script lang="ts">
  import { t } from '../../i18n/messages';
  import { onMount } from 'svelte';
  import { ExternalLink } from 'lucide-svelte';
  import { openReleasesPage, openNewIssue } from '../projectLinks';
  import { fetchVersionInfo } from '../../api/settings';
  import { updateStatus } from '../../stores/appState';

  // The three version numbers are read here rather than passed down: they are this section's own
  // data, nothing else in the dialog uses them, and the dialog is already the largest component in
  // the app without holding state on behalf of one tab.
  export let updateCheckOnStartup = true;

  let appVersion = '';
  let coreVersion = '';
  let pluginApiVersion = '';

  onMount(async () => {
    const version = await fetchVersionInfo();
    if (!version) return;
    appVersion = version.appVersion;
    coreVersion = version.coreVersion;
    pluginApiVersion = version.pluginApiVersion;
  });
</script>

<div class="section">
  <h4>xQuakShell</h4>

  <div class="version-table">
    <div class="version-row">
      <span class="version-label">{$t('about.version')}</span>
      <span class="version-value">{appVersion || '—'}</span>
    </div>
    <div class="version-row">
      <span class="version-label">{$t('about.core')}</span>
      <span class="version-value">{coreVersion || '—'}</span>
    </div>
    <div class="version-row">
      <span class="version-label">{$t('about.pluginApi')}</span>
      <span class="version-value">{pluginApiVersion || '—'}</span>
    </div>
  </div>

  {#if $updateStatus.updateAvailable}
    <p class="update-line update-available">
      {$t('about.updateAvailable', { version: $updateStatus.latestVersion })}
    </p>
  {:else if $updateStatus.checked}
    <p class="update-line">{$t('about.upToDate')}</p>
  {/if}

  <label class="checkbox-row">
    <input type="checkbox" bind:checked={updateCheckOnStartup} />
    {$t('about.checkOnStartup')}
  </label>

  <div class="about-links">
    <button class="secondary about-link" on:click={() => openReleasesPage()}>
      <ExternalLink size={13} />
      {$t('about.releases')}
    </button>
    <button class="secondary about-link" on:click={() => openNewIssue()}>
      <ExternalLink size={13} />
      {$t('about.reportIssue')}
    </button>
  </div>
</div>

<style>
  .version-table {
    display: flex;
    flex-direction: column;
    gap: 3px;
  }

  .update-line {
    margin: 0;
    font-size: 11px;
    color: var(--text-secondary);
  }

  .update-available {
    color: var(--accent);
    font-weight: 600;
  }
</style>
