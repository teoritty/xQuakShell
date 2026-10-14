<script lang="ts">
  // A team's row in the tree. The items beneath it are that team's, shared with everyone in it; the
  // row is where to add one there, and where to drop connections to move them in. It is not a
  // folder: it cannot be renamed, deleted or dragged here, and it takes no part in the selection -
  // a team row in a selection that Delete acts on would ask what deleting a team means.
  import { ChevronDown, ChevronRight, FolderPlus, Plus, Users } from 'lucide-svelte';
  import { t } from '../../i18n/messages';
  import { connections, folders } from '../../stores/appState';
  import { collapsedVaults } from '../../stores/teamState';
  import { createNewConnectionInFolder, moveConnections } from '../../actions/connectionActions';
  import { createFolderAndReveal, moveFolders } from '../../actions/folderActions';
  import { parseDragPayload } from './dndHandlers';
  import { redirectAcrossVaults } from './vaultDnd';
  import type { TreeNode, VaultGroup } from './types';

  export let node: TreeNode;

  let dropping = false;
  $: group = node.group as VaultGroup;
  $: title = group.name || $t('sync.teams.unnamed');

  function toggle() {
    collapsedVaults.update((set) => {
      const next = new Set(set);
      if (next.has(group.vault)) next.delete(group.vault);
      else next.add(group.vault);
      return next;
    });
  }

  // Both handlers stop the event: left to bubble, a drop here reaches the tree's own drop zone, which
  // reads as the personal vault's top level and would move the connections out of every team.
  function dragOver(e: DragEvent) {
    e.stopPropagation();
    if (!group.writable) return;
    e.preventDefault();
    dropping = true;
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
  }

  function drop(e: DragEvent) {
    e.preventDefault();
    e.stopPropagation();
    dropping = false;
    const payload = parseDragPayload(e.dataTransfer?.getData('application/json') ?? '');
    if (!payload || !group.writable) return;
    // A move from another vault goes to the confirmation; within the team it is a move to its top.
    if (redirectAcrossVaults(payload, group.vault, '', $connections, $folders)) return;
    if (payload.connectionIds.length > 0) void moveConnections(payload.connectionIds, '');
    void moveFolders(payload.folderIds, '');
  }
</script>

<div
  class="tree-node vault-row"
  class:drop-target={dropping}
  role="treeitem"
  aria-selected="false"
  aria-expanded={!!node.expanded}
  tabindex="0"
  title={group.viewer ? $t('tree.vault.viewerHint') : $t('tree.vault.hint')}
  on:click={toggle}
  on:keydown={(e) => (e.key === 'Enter' || e.key === ' ') && (e.preventDefault(), toggle())}
  on:contextmenu|preventDefault|stopPropagation
  on:dragover={dragOver}
  on:dragleave={() => (dropping = false)}
  on:drop={drop}
>
  <span class="folder-arrow">
    {#if node.expanded}<ChevronDown size={12} />{:else}<ChevronRight size={12} />{/if}
  </span>
  <span class="folder-icon"><Users size={14} /></span>
  <span class="node-name">{title}</span>
  {#if group.viewer}
    <span class="protocol-badge">{$t('tree.vault.viewOnly')}</span>
  {/if}
  {#if group.writable}
    <div class="folder-actions">
      <button class="micro-btn" on:click|stopPropagation={() => createNewConnectionInFolder('', group.vault)} title={$t('tree.action.newConnection')}>
        <Plus size={12} />
      </button>
      <button class="micro-btn" on:click|stopPropagation={() => createFolderAndReveal('', group.vault)} title={$t('tree.action.newFolder')}>
        <FolderPlus size={12} />
      </button>
    </div>
  {/if}
</div>

<style>
  .vault-row {
    margin-top: 6px;
    font-weight: 600;
    border-top: 1px solid var(--border-color);
  }
</style>
