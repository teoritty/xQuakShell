<script lang="ts">
  import RemoteTree from './RemoteTree.svelte';
  import ConnectionDetails from './ConnectionDetails.svelte';
</script>

<div class="sidebar">
  <RemoteTree />
  <ConnectionDetails />
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
