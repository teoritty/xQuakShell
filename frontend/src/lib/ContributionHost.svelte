<script lang="ts">
  import { onMount } from 'svelte';
  import { pluginContributions, refreshPluginContributions } from '../stores/pluginState';
  import PluginWebViewPanel from './PluginWebViewPanel.svelte';

  $: sidebarViews = ($pluginContributions.views || []).filter(v => v.location === 'sidebar.bottom');

  onMount(() => {
    void refreshPluginContributions();
  });
</script>

{#each sidebarViews as view (view.fullId)}
  {#if view.type === 'webview'}
    <PluginWebViewPanel
      pluginId={view.pluginId}
      panelId={view.id}
      assetUrl={view.assetUrl}
      title={view.title}
    />
  {/if}
{/each}
