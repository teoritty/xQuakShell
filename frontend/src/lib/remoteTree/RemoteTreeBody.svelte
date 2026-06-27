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
  {#each flatNodes as node (node.type + '-' + node.id)}
    <RemoteTreeNode
      {node}
      selected={selectedPaths.has(node.id)}
      ariaSelected={selectedPaths.has(node.id)}
      draggable={true}
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
    />
  {/each}
  {#if flatNodes.length === 0}
    <div class="empty-tree">No connections yet</div>
  {/if}
</div>
