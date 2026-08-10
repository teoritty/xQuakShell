<script lang="ts">
  import { X, ArrowUpCircle } from 'lucide-svelte';
  import { openReleasesPage } from './projectLinks';
  import { updateStatus } from '../stores/appState';

  // Dismissal is per run, not persisted. The banner only appears when the running release is no
  // longer supported, so remembering the dismissal would quietly retire the one signal the support
  // policy depends on; the next launch is cheap enough to ask again.
  let dismissed = false;

  $: show = $updateStatus.updateAvailable && !dismissed;
</script>

{#if show}
  <div class="update-banner" role="status">
    <ArrowUpCircle size={15} />
    <span>
      Version {$updateStatus.latestVersion} is available — you are running
      {$updateStatus.currentVersion}. Only the latest release receives fixes.
    </span>
    <button class="link" on:click={() => openReleasesPage()}>Open releases</button>
    <button class="dismiss" aria-label="Dismiss" on:click={() => (dismissed = true)}>
      <X size={14} />
    </button>
  </div>
{/if}

<style>
  .update-banner {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 6px 12px;
    font-size: 12.5px;
    background: var(--panel-alt, #22303f);
    border-bottom: 1px solid var(--border, #33455a);
    color: var(--fg, #cfd8e3);
  }

  .update-banner span {
    flex: 1;
    min-width: 0;
  }

  .link {
    background: none;
    border: none;
    padding: 0;
    color: var(--accent, #4aa3ff);
    cursor: pointer;
    text-decoration: underline;
    font-size: inherit;
  }

  .dismiss {
    background: none;
    border: none;
    padding: 2px;
    display: flex;
    color: inherit;
    opacity: 0.7;
    cursor: pointer;
  }

  .dismiss:hover {
    opacity: 1;
  }
</style>
