<script lang="ts">
  import type { RemoteNode } from '../stores/appState';
  import { Folder as FolderIcon, FolderOpen, File, Loader2, ChevronRight, ChevronDown } from 'lucide-svelte';

  export let node: RemoteNode;
  export let tree: Map<string, RemoteNode[]>;
  export let expanded: Set<string>;
  export let loading: Set<string>;
  export let sessionId: string;
  export let selectedPaths: Set<string> = new Set();
  export let onToggle: (path: string) => void;
  export let onSelect: (path: string, e?: MouseEvent) => void;
  export let onNavigate: (path: string) => void;
  export let onDrop: (e: DragEvent, targetDir: string) => void;
  export let onDragOverPath: ((e: DragEvent, path: string) => void) | undefined = undefined;
  export let dropTargetPath: string | null = null;
  export let onDragStartFile: (e: DragEvent, n: RemoteNode) => void;
  export let onContextMenu: ((e: MouseEvent, n: RemoteNode) => void) | undefined = undefined;
  export let formatSize: (size: number) => string;
  export let showPermissions = false;
  export let showOwner = false;
  export let showDate = false;
  export let editingNewPath: string | null = null;
  export let onRenameConfirm: ((oldPath: string, newName: string) => void) | undefined = undefined;
  export let onRenameCancel: (() => void) | undefined = undefined;

  let editValue = '';
  $: if (editingNewPath === node.path) {
    editValue = node.name;
  }
</script>

<div class="tree-node" class:dir={node.isDir}>
  <div
    class="node-row"
    class:selected={selectedPaths.has(node.path)}
    class:drop-target={node.isDir && dropTargetPath === node.path}
    on:click={(e) => onSelect(node.path, e)}
    on:dblclick={() => node.isDir && onNavigate(node.path)}
    on:contextmenu={(e) => onContextMenu?.(e, node)}
    on:keydown={(e) => {
      if (e.key === 'Enter') node.isDir ? onNavigate(node.path) : onSelect(node.path, e);
    }}
    draggable={true}
    on:dragstart={(e) => onDragStartFile(e, node)}
    on:dragover={node.isDir && onDragOverPath ? (e) => onDragOverPath(e, node.path) : undefined}
    on:drop={node.isDir ? (e) => onDrop(e, node.path) : undefined}
    role="treeitem"
    aria-selected={selectedPaths.has(node.path)}
    tabindex="0"
  >
    {#if node.isDir}
      <span class="folder-arrow" role="button" tabindex="-1" on:click|stopPropagation={() => onToggle(node.path)} on:keydown|stopPropagation={(e) => e.key === 'Enter' && onToggle(node.path)}>
        {#if expanded.has(node.path)}<ChevronDown size={12} />{:else}<ChevronRight size={12} />{/if}
      </span>
    {/if}
    <span class="node-icon">
      {#if node.isDir}
        {#if expanded.has(node.path)}<FolderOpen size={13} />{:else}<FolderIcon size={13} />{/if}
      {:else}
        <File size={13} />
      {/if}
    </span>
    {#if editingNewPath === node.path && onRenameConfirm}
      <input
        class="inline-edit-input"
        type="text"
        bind:value={editValue}
        autofocus
        on:blur={() => onRenameConfirm(node.path, editValue)}
        on:keydown={(e) => {
          if (e.key === 'Enter') onRenameConfirm(node.path, editValue);
          if (e.key === 'Escape') onRenameCancel?.();
        }}
        on:click|stopPropagation
      />
    {:else}
      <span class="node-name">{node.name}</span>
    {/if}
    {#if !node.isDir}
      <span class="node-size">{formatSize(node.size)}</span>
    {/if}
    {#if showPermissions && node.mode}
      <span class="node-col mode">{node.mode}</span>
    {/if}
    {#if showOwner && (node.owner || node.group)}
      <span class="node-col owner">{node.owner || '-'}/{node.group || '-'}</span>
    {/if}
    {#if showDate && node.modTime}
      <span class="node-col date">{node.modTime}</span>
    {/if}
    {#if loading.has(node.path)}
      <span class="node-loading"><Loader2 size={10} /></span>
    {/if}
  </div>

  {#if node.isDir && expanded.has(node.path)}
    <div class="node-children">
      {#each tree.get(node.path) || [] as child (child.path)}
        <svelte:self
          node={child}
          {tree}
          {expanded}
          {loading}
          {sessionId}
          {selectedPaths}
          onToggle={onToggle}
          onSelect={onSelect}
          onNavigate={onNavigate}
          onDrop={onDrop}
          onDragOverPath={onDragOverPath}
          dropTargetPath={dropTargetPath}
          onDragStartFile={onDragStartFile}
          onContextMenu={onContextMenu}
          formatSize={formatSize}
          {showPermissions}
          {showOwner}
          {showDate}
          {editingNewPath}
          onRenameConfirm={onRenameConfirm}
          onRenameCancel={onRenameCancel}
        />
      {/each}
    </div>
  {/if}
</div>

<style>
  .tree-node { user-select: none; }

  .node-row {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 2px 8px;
    cursor: pointer;
    font-size: 12px;
    transition: background 0.1s;
  }

  .node-row:hover { background: var(--bg-hover); }
  .node-row.selected { background: var(--bg-selected, rgba(255, 255, 255, 0.08)); }
  .node-row.drop-target { background: rgba(100, 150, 255, 0.2); outline: 1px dashed var(--accent); }
  .folder-arrow {
    display: inline-flex;
    align-items: center;
    flex-shrink: 0;
    cursor: pointer;
    color: var(--text-secondary);
  }
  .folder-arrow:hover { color: var(--text-primary); }

  .node-icon {
    display: inline-flex;
    align-items: center;
    flex-shrink: 0;
    color: var(--text-secondary);
  }

  .node-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .inline-edit-input {
    flex: 1;
    min-width: 0;
    padding: 0 2px;
    font-size: inherit;
    color: inherit;
    background: var(--bg-secondary);
    border: 1px solid var(--accent);
    border-radius: 2px;
    outline: none;
  }

  .node-size {
    font-size: 10px;
    color: var(--text-secondary);
    flex-shrink: 0;
  }

  .node-col {
    font-size: 10px;
    color: var(--text-secondary);
    flex-shrink: 0;
    max-width: 120px;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .node-loading {
    display: inline-flex;
    align-items: center;
    flex-shrink: 0;
    color: var(--text-secondary);
  }

  .node-children { padding-left: 16px; }
</style>
