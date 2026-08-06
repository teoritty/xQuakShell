<script lang="ts">
  import RemoteTree from './RemoteTree.svelte';
  import ConnectionDetails from './ConnectionDetails.svelte';
  import ContributionHost from './ContributionHost.svelte';
  import DiscoveryNodeDetails from './DiscoveryNodeDetails.svelte';
</script>

<div class="sidebar">
  <RemoteTree />
  <ConnectionDetails />
  <!-- The other kind of tree row's details. Only one of the two is ever open: selecting a node
       clears the connection selection and the other way round. -->
  <DiscoveryNodeDetails />
  <ContributionHost />
</div>

<style>
  .sidebar {
    display: flex;
    flex-direction: column;
    width: 280px;
    min-width: 200px;
    max-width: 450px;
    background: var(--bg-secondary);
    border-right: 1px solid var(--border-color);
    overflow: hidden;
    flex-shrink: 0;
  }
</style>
