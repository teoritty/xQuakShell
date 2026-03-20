<script lang="ts">
  import { folders } from '../stores/appState';
  import type { Folder } from '../stores/appState';
  import { writable } from 'svelte/store';
  const selectedFolderId = writable<string>('');
  import { saveFolder, deleteFolder } from '../stores/api';

  let newFolderName = '';
  let editingId: string | null = null;
  let editingName = '';

  function selectFolder(id: string) {
    selectedFolderId.set(id === $selectedFolderId ? '' : id);
  }

  async function createFolder() {
    if (!newFolderName.trim()) return;
    await saveFolder({ name: newFolderName.trim(), parentId: '', order: $folders.length });
    newFolderName = '';
  }

  function startEdit(folder: Folder) {
    editingId = folder.id;
    editingName = folder.name;
  }

  async function finishEdit() {
    if (editingId && editingName.trim()) {
      await saveFolder({ id: editingId, name: editingName.trim() });
    }
    editingId = null;
    editingName = '';
  }

  async function removeFolderConfirm(id: string) {
    if (confirm('Delete this folder and move its connections to root?')) {
      await deleteFolder(id);
      if ($selectedFolderId === id) selectedFolderId.set('');
    }
  }

  function handleDrop(e: DragEvent, folderId: string) {
    e.preventDefault();
    const data = e.dataTransfer?.getData('text/plain');
    if (data) {
      const connectionIds = JSON.parse(data) as string[];
      import('../stores/api').then(api => api.moveConnections(connectionIds, folderId));
    }
  }

  function handleDragOver(e: DragEvent) {
    e.preventDefault();
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
  }
</script>

<div class="folder-panel">
  <div class="panel-header">
    <span>Folders</span>
    <div class="actions">
      <button on:click={() => document.getElementById('new-folder-input')?.focus()} title="New folder">+</button>
    </div>
  </div>

  <div class="folder-create">
    <input
      id="new-folder-input"
      type="text"
      placeholder="New folder..."
      bind:value={newFolderName}
      on:keydown={(e) => e.key === 'Enter' && createFolder()}
    />
    <button on:click={createFolder} disabled={!newFolderName.trim()}>Add</button>
  </div>

  <div class="folder-list">
    <div
      class="folder-item"
      class:active={$selectedFolderId === ''}
      on:click={() => selectFolder('')}
      on:keydown={(e) => e.key === 'Enter' && selectFolder('')}
      on:drop={(e) => handleDrop(e, '')}
      on:dragover={handleDragOver}
      role="button"
      tabindex="0"
    >
      <span class="folder-icon">📁</span>
      <span class="folder-name">All Connections</span>
    </div>

    {#each $folders as folder (folder.id)}
      <div
        class="folder-item"
        class:active={$selectedFolderId === folder.id}
        on:click={() => selectFolder(folder.id)}
        on:keydown={(e) => e.key === 'Enter' && selectFolder(folder.id)}
        on:drop={(e) => handleDrop(e, folder.id)}
        on:dragover={handleDragOver}
        on:dblclick={() => startEdit(folder)}
        role="button"
        tabindex="0"
      >
        <span class="folder-icon">📂</span>
        {#if editingId === folder.id}
          <input
            class="folder-edit-input"
            type="text"
            bind:value={editingName}
            on:blur={finishEdit}
            on:keydown={(e) => e.key === 'Enter' && finishEdit()}
          />
        {:else}
          <span class="folder-name">{folder.name}</span>
        {/if}
        <button class="folder-delete" on:click|stopPropagation={() => removeFolderConfirm(folder.id)} title="Delete folder">✕</button>
      </div>
    {/each}
  </div>
</div>

<style>
  .folder-panel {
    display: flex;
    flex-direction: column;
    border-bottom: 1px solid var(--border-color);
  }

  .folder-create {
    display: flex;
    gap: 4px;
    padding: 6px 8px;
    border-bottom: 1px solid var(--border-color);
  }

  .folder-create input {
    flex: 1;
    min-width: 0;
  }

  .folder-create button {
    padding: 4px 8px;
    font-size: 11px;
  }

  .folder-list {
    overflow-y: auto;
    max-height: 200px;
  }

  .folder-item {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 5px 10px;
    cursor: pointer;
    user-select: none;
    transition: background 0.1s;
  }

  .folder-item:hover {
    background: var(--bg-hover);
  }

  .folder-item.active {
    background: var(--bg-active);
    color: var(--text-bright);
  }

  .folder-icon {
    font-size: 14px;
    flex-shrink: 0;
  }

  .folder-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: 12px;
  }

  .folder-edit-input {
    flex: 1;
    min-width: 0;
    font-size: 12px;
    padding: 1px 4px;
  }

  .folder-delete {
    display: none;
    background: transparent;
    color: var(--text-secondary);
    padding: 0 4px;
    font-size: 10px;
  }

  .folder-item:hover .folder-delete {
    display: inline;
  }

  .folder-delete:hover {
    color: var(--danger);
  }
</style>
