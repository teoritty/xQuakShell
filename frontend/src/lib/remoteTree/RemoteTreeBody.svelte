<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import RemoteTreeNode from './RemoteTreeNode.svelte';
  import RemoteTreeFavorites from './RemoteTreeFavorites.svelte';
  import type { Connection } from '../../stores/appState';
  import type { ConnectionStatus, DropZone, TreeNode } from './types';
  import './remoteTreeShared.css';

  export let flatNodes: TreeNode[] = [];
  export let favoriteConns: Connection[] = [];
  export let selectedPaths: Set<string> = new Set();
  export let selectedConnectionCount = 0;
  export let dragOverDropZone: DropZone | null = null;
  export let dragOverTargetId: string | null = null;
  export let editingFolderId: string | null = null;
  export let editingConnId: string | null = null;
  export let editingFolderName = '';
  export let editingConnName = '';
  export let pingResults: Map<string, { reachable?: boolean; latencyMs?: number }> = new Map();
  export let sessionStatusByConnId: Map<string, ConnectionStatus> = new Map();
  /** Connections that can have a discovery subtree at all (they have a session). */
  export let discoveryAvailableIds: Set<string> = new Set();
  export let discoveryIcons: Map<string, string> = new Map();
  /** discoveryKey values selected inside a subtree — a set of its own, never selectedPaths. */
  export let discoverySelectedKeys: Set<string> = new Set();
  export let searchHint = '';

  const dispatch = createEventDispatcher();
</script>

<div class="tree-body" role="tree" tabindex="0" on:keydown={(e) => dispatch('treeKeydown', { event: e })}>
  <RemoteTreeFavorites
    connections={favoriteConns}
    {selectedPaths}
    {pingResults}
    {sessionStatusByConnId}
    on:select={(e) => dispatch('selectConnection', e.detail)}
    on:open={(e) => dispatch('openConnection', e.detail)}
    on:contextmenu={(e) => dispatch('contextmenu', e.detail)}
  />
  {#if searchHint}
    <div class="search-hint">{searchHint}</div>
  {/if}
  <!-- node.id already carries connectionId, pluginId and nodeId for discovery
       rows, so two plugins publishing the same node id under one connection
       cannot collapse into a single keyed row. -->
  {#each flatNodes as node (node.type + '-' + node.id)}
    <RemoteTreeNode
      {node}
      selected={node.type === 'discovery'
        ? !!node.discovery && discoverySelectedKeys.has(node.discovery.key)
        : selectedPaths.has(node.id)}
      ariaSelected={node.type === 'discovery'
        ? !!node.discovery && discoverySelectedKeys.has(node.discovery.key)
        : selectedPaths.has(node.id)}
      draggable={node.type !== 'discovery'}
      discoveryAvailable={node.type === 'connection' && discoveryAvailableIds.has(node.id)}
      {discoveryIcons}
      {dragOverDropZone}
      {dragOverTargetId}
      {editingFolderId}
      {editingConnId}
      bind:editingFolderName
      bind:editingConnName
      {pingResults}
      {sessionStatusByConnId}
      selectedConnectionCount={selectedConnectionCount}
      on:dragstart={(e) => dispatch('dragstart', e.detail)}
      on:dragend={() => dispatch('dragend')}
      on:dragover={(e) => dispatch('dragover', e.detail)}
      on:dragenter={(e) => dispatch('dragenter', e.detail)}
      on:dragleave={() => dispatch('dragleave')}
      on:drop={(e) => dispatch('drop', e.detail)}
      on:click={(e) => dispatch('nodeClick', e.detail)}
      on:dblclick={(e) => dispatch('nodeDblclick', e.detail)}
      on:contextmenu={(e) => dispatch('contextmenu', e.detail)}
      on:keydown={(e) => dispatch('nodeKeydown', e.detail)}
      on:toggleFolder={(e) => dispatch('toggleFolder', e.detail)}
      on:confirmRenameFolder={() => dispatch('confirmRenameFolder')}
      on:cancelRenameFolder={() => dispatch('cancelRenameFolder')}
      on:confirmRenameConnection={() => dispatch('confirmRenameConnection')}
      on:cancelRenameConnection={() => dispatch('cancelRenameConnection')}
      on:newSubfolder={(e) => dispatch('newSubfolder', e.detail)}
      on:startRenameFolder={(e) => dispatch('startRenameFolder', e.detail)}
      on:deleteFolder={(e) => dispatch('deleteFolder', e.detail)}
      on:startRenameConnection={(e) => dispatch('startRenameConnection', e.detail)}
      on:deleteConnection={(e) => dispatch('deleteConnection', e.detail)}
      on:toggleDiscoveryRoot={(e) => dispatch('toggleDiscoveryRoot', e.detail)}
      on:toggleDiscoveryNode={(e) => dispatch('toggleDiscoveryNode', e.detail)}
    />
  {/each}
  {#if flatNodes.length === 0}
    <div class="empty-tree">No connections yet</div>
  {/if}
</div>
