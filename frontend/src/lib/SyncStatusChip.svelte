<script lang="ts">
  // The sync indicator in the top bar. It is shown only when sync is enabled, so a user who never
  // turned sync on sees nothing new; it turns red when sync stopped, because a stopped vault needs
  // the person - a verification failure or keys rotated elsewhere - and will not fix itself.
  import { createEventDispatcher } from 'svelte';
  import { RefreshCw, AlertTriangle } from 'lucide-svelte';
  import { pluginDataNotices, syncStatus } from '../stores/syncState';
  import { vaultUnlocked } from '../stores/appState';
  import { refreshSyncStatus } from '../actions/syncActions';
  import { t } from '../i18n/messages';

  const dispatch = createEventDispatcher();

  // The account lives in the vault, so what to show is known only once it is open.
  $: if ($vaultUnlocked) void refreshSyncStatus();

  // A plugin data conflict also asks for the person: the losing version is in the trash only for the
  // retention window, so it is worth a look before it is gone.
  $: attention =
    $syncStatus.state === 'stopped' ||
    $syncStatus.held > 0 ||
    !!$syncStatus.newDevices?.length ||
    $pluginDataNotices.length > 0;
  $: title = $syncStatus.message || $t('sync.state.' + $syncStatus.state);
</script>

{#if $syncStatus.enabled}
  <button class="ghost chip" class:attention class:spinning={$syncStatus.state === 'running'} {title} on:click={() => dispatch('open')}>
    {#if attention}
      <AlertTriangle size={14} />
    {:else}
      <RefreshCw size={14} />
    {/if}
  </button>
{/if}

<style>
  .chip {
    padding: 4px 6px;
    border-radius: 2px;
    display: inline-flex;
    align-items: center;
  }

  .attention {
    color: var(--danger);
  }

  .spinning :global(svg) {
    animation: spin 1.2s linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
</style>
