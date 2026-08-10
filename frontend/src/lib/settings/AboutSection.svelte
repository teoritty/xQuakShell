<script lang="ts">
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
  <h4>SSH Client</h4>
  <p class="version-text">Version {appVersion || '—'}</p>
  <p class="version-text">Core {coreVersion || '—'} · Plugin API {pluginApiVersion || '—'}</p>

  {#if $updateStatus.updateAvailable}
    <p class="version-text update-available">
      Version {$updateStatus.latestVersion} is available.
    </p>
  {:else if $updateStatus.checked}
    <p class="version-text muted">You are on the latest release.</p>
  {/if}

  <label class="update-toggle">
    <input type="checkbox" bind:checked={updateCheckOnStartup} />
    Check for updates on startup
  </label>
  <p class="version-text muted">
    Only the latest release is supported. The check is one anonymous request to GitHub; nothing is
    downloaded or installed.
  </p>

  <div class="about-links">
    <button class="secondary about-link" on:click={() => openReleasesPage()}>
      <ExternalLink size={13} />
      Check for Updates
    </button>
    <button class="secondary about-link" on:click={() => openNewIssue()}>
      <ExternalLink size={13} />
      Report an Issue
    </button>
  </div>
</div>

<style>
  .update-available {
    color: var(--accent, #4aa3ff);
    font-weight: 600;
  }

  .muted {
    opacity: 0.7;
  }

  .update-toggle {
    display: flex;
    align-items: center;
    gap: 8px;
    margin: 10px 0 4px;
    cursor: pointer;
  }
</style>
