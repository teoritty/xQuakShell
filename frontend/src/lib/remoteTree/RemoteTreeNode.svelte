<script lang="ts">
  // Row dispatcher. It owns the wrapper element — indentation, indent guides,
  // selection and drop classes, and the pointer/keyboard/drag events, all of
  // which are identical for every row kind — and delegates the row's contents to
  // one of three components.
  //
  // The split happened when discovery added a fourth kind of row (ADR-014): one
  // component branching four ways on node.type would have been unreadable. The
  // folder and connection markup moved across unchanged; this is a refactor, not
  // a redesign.
  import { createEventDispatcher } from 'svelte';
  import RemoteTreeConnectionRow from './RemoteTreeConnectionRow.svelte';
  import RemoteTreeDiscoveryRow from './RemoteTreeDiscoveryRow.svelte';
  import RemoteTreeFolderRow from './RemoteTreeFolderRow.svelte';
  import { range } from './buildTree';
  import { isNodeEditing } from './dndGuards';
  import type { ConnectionStatus, DropZone, TreeNode } from './types';
  import './remoteTreeShared.css';

  export let node: TreeNode;
  export let selected = false;
  export let ariaSelected = false;
  export let draggable = true;
  export let dragOverDropZone: DropZone | null = null;
  export let dragOverTargetId: string | null = null;
  export let editingFolderId: string | null = null;
  export let editingConnId: string | null = null;
  export let editingFolderName = '';
  export let editingConnName = '';
  export let pingResults: Map<string, { reachable?: boolean; latencyMs?: number }> = new Map();
  export let sessionStatusByConnId: Map<string, ConnectionStatus> = new Map();
  export let selectedConnectionCount = 1;
  export let discoveryAvailable = false;
  export let discoveryIcons: Map<string, string> = new Map();

  const dispatch = createEventDispatcher();

  $: isDiscovery = node.type === 'discovery';
  // A service line (loading / error / "showing N of M") is host chrome, not a
  // resource: it cannot be selected, activated or acted on.
  $: isNotice = isDiscovery && !!node.notice;
  $: isEditing = isNodeEditing(node, editingFolderId, editingConnId);
  $: dropTarget = dragOverDropZone === 'folder' && dragOverTargetId === node.id;
  $: dropBefore = dragOverDropZone === 'before' && dragOverTargetId === node.id;
  $: dropAfter = dragOverDropZone === 'after' && dragOverTargetId === node.id;
</script>

<div
  class="tree-node"
  class:folder={node.type === 'folder'}
  class:connection={node.type === 'connection'}
  class:discovery={isDiscovery}
  class:discovery-notice-row={isNotice}
  class:discovery-stale={isDiscovery && !!node.discovery?.stale}
  class:selected
  class:drop-target={dropTarget}
  class:drop-target-before={dropBefore}
  class:drop-target-after={dropAfter}
  style="padding-left: calc({Math.min(8 + node.depth * 12, 100)}px * var(--ui-scale))"
  data-discovery-id={node.discovery ? node.id : null}
  draggable={draggable && !isEditing && !isDiscovery}
  role="treeitem"
  aria-selected={ariaSelected}
  tabindex="0"
  on:dragstart={(e) => !isEditing && !isDiscovery && dispatch('dragstart', { event: e, node })}
  on:dragend={() => dispatch('dragend')}
  on:dragover={(e) => dispatch('dragover', { event: e, node })}
  on:dragenter={() => node.type === 'folder' && dispatch('dragenter', { node })}
  on:dragleave={() => node.type === 'folder' && dispatch('dragleave')}
  on:drop={(e) => dispatch('drop', { event: e, node })}
  on:click={(e) => !isNotice && dispatch('click', { event: e, node })}
  on:dblclick={() => !isNotice && dispatch('dblclick', { node })}
  on:contextmenu={(e) => !isNotice && dispatch('contextmenu', { event: e, node })}
  on:keydown={(e) => dispatch('keydown', { event: e, node })}
>
  {#each range(node.depth) as l}
    <span class="indent-guide" style="left: calc({8 + l * 12 + 7}px * var(--ui-scale))"></span>
  {/each}
  {#if node.type === 'folder'}
    <RemoteTreeFolderRow
      {node}
      {editingFolderId}
      bind:editingFolderName
      on:toggleFolder
      on:confirmRenameFolder
      on:cancelRenameFolder
      on:newSubfolder
      on:startRenameFolder
      on:deleteFolder
    />
  {:else if node.type === 'connection'}
    <RemoteTreeConnectionRow
      {node}
      {selected}
      {editingConnId}
      bind:editingConnName
      {pingResults}
      {sessionStatusByConnId}
      {selectedConnectionCount}
      {discoveryAvailable}
      discoveryExpanded={!!node.expanded}
      on:toggleDiscoveryRoot
      on:confirmRenameConnection
      on:cancelRenameConnection
      on:startRenameConnection
      on:deleteConnection
    />
  {:else}
    <RemoteTreeDiscoveryRow {node} icons={discoveryIcons} on:toggleDiscoveryNode />
  {/if}
</div>
