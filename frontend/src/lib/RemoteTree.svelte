<script lang="ts">
  import {
    connections,
    detailsConnectionId,
    expandedFolderIds,
    favorites,
    folders,
    selectedConnectionId,
    selectedConnectionIds,
    selectedFolderId,
    sessions,
    pingResults,
    type Connection,
    type Folder,
  } from '../stores/appState';
  import {
    createNewConnectionInFolder,
    createNewFolderInFolder,
    deleteConnection,
    deleteFolder,
    openSession,
    saveConnection,
    saveFolder,
  } from '../stores/api';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import ImportPuTTYDialog from './ImportPuTTYDialog.svelte';
  import RemoteTreeContextMenu from './RemoteTreeContextMenu.svelte';
  import { buildTree, countConnectionsInFolder, flattenTree } from './remoteTree/buildTree';
  import { buildSessionStatusMap } from './remoteTree/connectionDisplay';
  import {
    computeDropZone,
    resolveDragPayload,
    shouldShowDropIndicator,
  } from './remoteTree/dndGuards';
  import {
    executeDropBetween,
    executeDropOnFolder,
    executeDropOnRootEnd,
    parseDragPayload,
  } from './remoteTree/dndHandlers';
  import RemoteTreeBody from './remoteTree/RemoteTreeBody.svelte';
  import RemoteTreeSearch from './remoteTree/RemoteTreeSearch.svelte';
  import RemoteTreeToolbar from './remoteTree/RemoteTreeToolbar.svelte';
  import {
    clearTreeSelection,
    connectionIdsForDelete,
    connectionIdsInSelection,
    folderIdsInSelection,
    prepareContextMenuSelection,
    selectTreeNode,
    syncSelectionStores,
    type SelectionStores,
  } from './remoteTree/selection';
  import { emptyDragVisualState, type DragPayload, type DragVisualState, type TreeNode } from './remoteTree/types';
  import { clampMenuPosition } from './clampMenuPosition';
  import './remoteTree/remoteTreeShared.css';

  const selectionStores: SelectionStores = {
    selectedConnectionId,
    selectedConnectionIds,
    selectedFolderId,
  };

  let searchQuery = '';
  let selectedPaths: Set<string> = new Set();
  let lastSelectedPath: string | null = null;
  let editingFolderId: string | null = null;
  let editingFolderName = '';
  let editingConnId: string | null = null;
  let editingConnName = '';
  let dragPayload: DragPayload | null = null;
  let dragVisual: DragVisualState = emptyDragVisualState();
  let ctxMenu = { show: false, x: 0, y: 0, node: null as TreeNode | null };
  let confirmDeleteShow = false;
  let confirmDeleteType: 'connection' | 'folder' = 'connection';
  let confirmDeleteId = '';
  let confirmDeleteIds: string[] = [];
  let confirmDeleteName = '';
  let confirmDeleteCritical = false;
  let confirmDeleteChildCount = 0;
  let showImportDialog = false;

  $: tree = buildTree($folders, $connections, $expandedFolderIds, searchQuery);
  $: flatNodes = flattenTree(tree);
  $: favoriteConns = $connections.filter((c) => $favorites.has(c.id));
  $: sessionStatusByConnId = buildSessionStatusMap($sessions);
  $: selectedConnectionCount = connectionIdsInSelection(selectedPaths, $connections).length;
  $: shiftNodes = (() => {
    const favIds = new Set(favoriteConns.map((c) => c.id));
    const favNodes: TreeNode[] = favoriteConns.map((c) => ({
      type: 'connection',
      id: c.id,
      name: c.name,
      depth: 0,
      parentId: '',
    }));
    return [...favNodes, ...flatNodes.filter((n) => !(n.type === 'connection' && favIds.has(n.id)))];
  })();

  function applySelection(result: { selectedPaths: Set<string>; lastSelectedPath: string }) {
    selectedPaths = result.selectedPaths;
    lastSelectedPath = result.lastSelectedPath;
    syncSelectionStores(selectedPaths, $connections, $folders, selectionStores);
  }

  function clearDragState() {
    dragVisual = emptyDragVisualState();
    dragPayload = null;
  }

  function toggleFolder(id: string) {
    expandedFolderIds.update((set) => {
      const next = new Set(set);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  function expandAll() {
    expandedFolderIds.set(new Set($folders.map((f) => f.id)));
  }

  function collapseAll() {
    expandedFolderIds.set(new Set());
  }

  function startRenameFolder(f: Folder) {
    editingFolderId = f.id;
    editingFolderName = f.name;
  }

  function startRenameConnection(c: Connection) {
    editingConnId = c.id;
    editingConnName = c.name;
  }

  async function confirmRenameFolder() {
    if (!editingFolderName.trim() || !editingFolderId) {
      editingFolderId = null;
      return;
    }
    const f = $folders.find((x) => x.id === editingFolderId);
    if (f) await saveFolder({ ...f, name: editingFolderName.trim() });
    editingFolderId = null;
  }

  async function confirmRenameConnection() {
    if (!editingConnName.trim() || !editingConnId) {
      editingConnId = null;
      return;
    }
    const c = $connections.find((x) => x.id === editingConnId);
    if (c) await saveConnection({ ...c, name: editingConnName.trim() });
    editingConnId = null;
  }

  function requestDeleteConnections(ids: string[]) {
    confirmDeleteType = 'connection';
    confirmDeleteIds = ids;
    confirmDeleteId = ids[0] || '';
    confirmDeleteName = ids.length === 1 ? ($connections.find((x) => x.id === ids[0])?.name ?? '') : '';
    confirmDeleteCritical = ids.length > 1;
    confirmDeleteChildCount = ids.length;
    confirmDeleteShow = true;
  }

  function requestDeleteFolder(f: Folder) {
    const childCount = countConnectionsInFolder(f.id, $folders, $connections);
    confirmDeleteType = 'folder';
    confirmDeleteId = f.id;
    confirmDeleteName = f.name;
    confirmDeleteCritical = childCount > 0;
    confirmDeleteChildCount = childCount;
    confirmDeleteShow = true;
  }

  function showContextMenu(e: MouseEvent, node: TreeNode) {
    e.preventDefault();
    e.stopPropagation();
    const prep = prepareContextMenuSelection(node, selectedPaths);
    if (prep) applySelection(prep);
    const pos = clampMenuPosition(
      { left: e.clientX, top: e.clientY, right: e.clientX, bottom: e.clientY },
      200,
      node.type === 'folder' ? 150 : 160
    );
    ctxMenu = { show: true, x: pos.left, y: pos.top, node };
  }

  function closeContextMenu() {
    ctxMenu = { ...ctxMenu, show: false };
  }

  function handleWindowClick(e: MouseEvent) {
    closeContextMenu();
    const target = e.target as HTMLElement | null;
    if (!target) return;
    if (target.closest('.tree-node')) return;
    if (target.closest('.context-menu')) return;
    if (selectedPaths.size === 0) return;
    selectedPaths = clearTreeSelection(selectionStores);
    lastSelectedPath = null;
  }

  function toggleFavorite(connId: string) {
    favorites.update((s) => {
      const next = new Set(s);
      if (next.has(connId)) next.delete(connId);
      else next.add(connId);
      return next;
    });
    closeContextMenu();
  }

  async function handleCtxDelete() {
    if (!ctxMenu.node) return;
    closeContextMenu();
    if (ctxMenu.node.type === 'connection') {
      requestDeleteConnections(connectionIdsForDelete(ctxMenu.node.id, selectedPaths, $connections));
    } else if (ctxMenu.node.folder) {
      requestDeleteFolder(ctxMenu.node.folder);
    }
  }

  async function handleCtxNewConnection() {
    const folderId = ctxMenu.node?.type === 'folder' ? ctxMenu.node.id : '';
    closeContextMenu();
    const saved = await createNewConnectionInFolder(folderId);
    if (saved) startRenameConnection(saved);
  }

  async function handleCtxNewFolder() {
    const folderId = ctxMenu.node?.type === 'folder' ? ctxMenu.node.id : '';
    closeContextMenu();
    await createNewFolderInFolder(folderId);
  }

  function handleCtxEdit() {
    if (!ctxMenu.node) return;
    closeContextMenu();
    if (ctxMenu.node.type === 'folder' && ctxMenu.node.folder) {
      startRenameFolder(ctxMenu.node.folder);
    } else if (ctxMenu.node.connection) {
      startRenameConnection(ctxMenu.node.connection);
    }
  }

  async function handleConfirmDelete() {
    confirmDeleteShow = false;
    if (confirmDeleteType === 'connection') {
      for (const id of confirmDeleteIds) await deleteConnection(id);
      selectedPaths = new Set();
      syncSelectionStores(selectedPaths, $connections, $folders, selectionStores);
      if (confirmDeleteIds.includes($selectedConnectionId)) selectedConnectionId.set('');
      if (confirmDeleteIds.includes($detailsConnectionId)) detailsConnectionId.set('');
    } else {
      await deleteFolder(confirmDeleteId);
      selectedPaths = new Set([...selectedPaths].filter((id) => id !== confirmDeleteId));
      syncSelectionStores(selectedPaths, $connections, $folders, selectionStores);
    }
  }

  function handleDragOver(e: DragEvent) {
    e.preventDefault();
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
  }

  function handleNodeDragOver(e: DragEvent, node: TreeNode) {
    e.preventDefault();
    e.stopPropagation();
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
    if (!dragPayload) return;
    const zone = computeDropZone(e, node);
    if (!shouldShowDropIndicator(dragPayload, node, zone, $connections, $folders, flatNodes)) {
      dragVisual = { ...dragVisual, dragOverDropZone: null, dragOverTargetId: null, dragOverId: null };
      return;
    }
    if (zone === 'folder' && node.type === 'folder') {
      dragVisual = {
        dragOverRoot: false,
        dragOverDropZone: 'folder',
        dragOverTargetId: node.id,
        dragOverId: node.id,
      };
    } else {
      dragVisual = {
        dragOverRoot: false,
        dragOverDropZone: zone,
        dragOverTargetId: node.id,
        dragOverId: null,
      };
    }
  }

  async function handleNodeDrop(e: DragEvent, node: TreeNode) {
    e.preventDefault();
    e.stopPropagation();
    const payload = dragPayload ?? parseDragPayload(e.dataTransfer?.getData('application/json') ?? '');
    if (!payload) {
      clearDragState();
      return;
    }
    const zone = computeDropZone(e, node);
    try {
      if (zone === 'folder' && node.type === 'folder') {
        await executeDropOnFolder(payload, node.id, $connections, $folders);
      } else if (zone === 'before' || zone === 'after') {
        await executeDropBetween(payload, node, zone, flatNodes, $connections, $folders);
      }
    } catch {
      // api.ts -> errorStore
    }
    clearDragState();
  }

  async function handleDropOnRoot(e: DragEvent) {
    e.preventDefault();
    e.stopPropagation();
    const payload = dragPayload ?? parseDragPayload(e.dataTransfer?.getData('application/json') ?? '');
    if (!payload) {
      clearDragState();
      return;
    }
    try {
      await executeDropOnRootEnd(payload, flatNodes, $connections, $folders);
    } catch {
      // api.ts -> errorStore
    }
    clearDragState();
  }

  function handleRootDragLeave(e: DragEvent) {
    const rt = e.relatedTarget as HTMLElement | null;
    if (!rt || !e.currentTarget || !(e.currentTarget as HTMLElement).contains(rt)) {
      dragVisual = { ...dragVisual, dragOverRoot: false, dragOverDropZone: null, dragOverTargetId: null };
    }
  }

  function handleSearchFocus() {
    selectedPaths = clearTreeSelection(selectionStores);
    lastSelectedPath = null;
  }

  function handleTreeKeydown(e: CustomEvent<{ event: KeyboardEvent }>) {
    const ev = e.detail.event;
    const target = ev.target as HTMLElement | null;
    const tag = target?.tagName ?? '';
    if (tag === 'INPUT' || tag === 'TEXTAREA' || target?.isContentEditable) return;
    if (ev.key !== 'Delete') return;
    const ids = connectionIdsInSelection(selectedPaths, $connections);
    if (ids.length > 0) {
      ev.preventDefault();
      requestDeleteConnections(ids);
    }
  }

  function handleSelectNode(id: string, e?: MouseEvent) {
    applySelection(selectTreeNode(id, shiftNodes, lastSelectedPath, selectedPaths, e));
    const isPlainClick = !e?.ctrlKey && !e?.metaKey && !e?.shiftKey;
    if (isPlainClick && $connections.some((c) => c.id === id)) {
      detailsConnectionId.set(id);
    }
  }

  function handleDragStart(e: CustomEvent<{ event: DragEvent; node: TreeNode }>) {
    const { event, node } = e.detail;
    if (!event.dataTransfer) return;
    const folderIds = new Set(folderIdsInSelection(selectedPaths, $folders));
    const connIds = new Set(connectionIdsInSelection(selectedPaths, $connections));
    dragPayload = resolveDragPayload(node, selectedPaths, folderIds, connIds);
    event.dataTransfer.effectAllowed = 'move';
    event.dataTransfer.setData('application/json', JSON.stringify(dragPayload));
  }

  function handleNodeDragLeave() {
    dragVisual = { ...dragVisual, dragOverId: null, dragOverDropZone: null, dragOverTargetId: null };
  }

  function handleNodeClick(e: CustomEvent<{ event: MouseEvent; node: TreeNode }>) {
    handleSelectNode(e.detail.node.id, e.detail.event);
  }

  function handleNodeDblclick(e: CustomEvent<{ node: TreeNode }>) {
    if (e.detail.node.connection) openSession(e.detail.node.connection.id);
    else toggleFolder(e.detail.node.id);
  }

  function handleNodeKeydown(e: CustomEvent<{ event: KeyboardEvent; node: TreeNode }>) {
    const { event, node } = e.detail;
    if (event.key === 'Enter' && node.connection) openSession(node.connection.id);
    if (event.key === 'Enter' && node.type === 'folder') toggleFolder(node.id);
  }

  function handleDeleteConnection(e: CustomEvent<{ connection?: Connection; multi?: boolean }>) {
    if (!e.detail.connection) return;
    if (e.detail.multi) requestDeleteConnections(connectionIdsInSelection(selectedPaths, $connections));
    else requestDeleteConnections([e.detail.connection.id]);
  }

  function handleRootDragEnter() {
    dragVisual = { ...dragVisual, dragOverRoot: true, dragOverId: null };
  }
</script>

<svelte:window on:click={handleWindowClick} />

<div
  class="remote-tree"
  class:drag-over-root={dragVisual.dragOverRoot}
  on:dragover={handleDragOver}
  on:drop={handleDropOnRoot}
  on:dragenter={handleRootDragEnter}
  on:dragleave={handleRootDragLeave}
>
  <RemoteTreeSearch bind:value={searchQuery} onFocus={handleSearchFocus} />
  <RemoteTreeToolbar
    onNewConnection={() => createNewConnectionInFolder($selectedFolderId)}
    onNewFolder={() => createNewFolderInFolder($selectedFolderId)}
    onImport={() => (showImportDialog = true)}
    onExpandAll={expandAll}
    onCollapseAll={collapseAll}
  />
  <RemoteTreeBody
    {flatNodes}
    {favoriteConns}
    {selectedPaths}
    {selectedConnectionCount}
    dragOverDropZone={dragVisual.dragOverDropZone}
    dragOverTargetId={dragVisual.dragOverTargetId}
    {editingFolderId}
    {editingConnId}
    bind:editingFolderName
    bind:editingConnName
    pingResults={$pingResults}
    {sessionStatusByConnId}
    on:treeKeydown={handleTreeKeydown}
    on:selectConnection={({ detail }) => handleSelectNode(detail.id, detail.event)}
    on:openConnection={({ detail }) => openSession(detail.connection.id)}
    on:contextmenu={({ detail }) => showContextMenu(detail.event, detail.node)}
    on:dragstart={handleDragStart}
    on:dragend={clearDragState}
    on:dragover={({ detail }) => handleNodeDragOver(detail.event, detail.node)}
    on:dragenter={() => {}}
    on:dragleave={handleNodeDragLeave}
    on:drop={({ detail }) => handleNodeDrop(detail.event, detail.node)}
    on:nodeClick={handleNodeClick}
    on:nodeDblclick={handleNodeDblclick}
    on:nodeKeydown={handleNodeKeydown}
    on:toggleFolder={({ detail }) => toggleFolder(detail.id)}
    on:confirmRenameFolder={confirmRenameFolder}
    on:cancelRenameFolder={() => (editingFolderId = null)}
    on:confirmRenameConnection={confirmRenameConnection}
    on:cancelRenameConnection={() => (editingConnId = null)}
    on:newSubfolder={({ detail }) => createNewFolderInFolder(detail.folderId)}
    on:startRenameFolder={({ detail }) => detail.folder && startRenameFolder(detail.folder)}
    on:deleteFolder={({ detail }) => detail.folder && requestDeleteFolder(detail.folder)}
    on:startRenameConnection={({ detail }) => detail.connection && startRenameConnection(detail.connection)}
    on:deleteConnection={handleDeleteConnection}
  />
</div>

<RemoteTreeContextMenu
  x={ctxMenu.x}
  y={ctxMenu.y}
  show={ctxMenu.show}
  isFolder={ctxMenu.node?.type === 'folder'}
  isConnection={ctxMenu.node?.type === 'connection'}
  isFavorite={ctxMenu.node?.type === 'connection' && ctxMenu.node?.id ? $favorites.has(ctxMenu.node.id) : false}
  on:delete={handleCtxDelete}
  on:edit={handleCtxEdit}
  on:newConnection={handleCtxNewConnection}
  on:newFolder={handleCtxNewFolder}
  on:toggleFavorite={() => ctxMenu.node?.type === 'connection' && toggleFavorite(ctxMenu.node.id)}
/>

<ConfirmDialog
  show={confirmDeleteShow}
  title={confirmDeleteType === 'folder' && confirmDeleteCritical
    ? 'Warning: Folder Contains Connections'
    : confirmDeleteType === 'connection' && confirmDeleteIds.length > 1
      ? 'Delete Multiple Connections'
      : `Delete ${confirmDeleteType === 'folder' ? 'Folder' : 'Connection'}`}
  message={confirmDeleteType === 'folder' && confirmDeleteCritical
    ? `You are about to delete folder "${confirmDeleteName}" which contains ${confirmDeleteChildCount} connection(s). This action cannot be undone!`
    : confirmDeleteType === 'connection' && confirmDeleteIds.length > 1
      ? `You are about to delete ${confirmDeleteIds.length} connection(s). This action cannot be undone!`
      : `Are you sure you want to delete "${confirmDeleteName}"?`}
  critical={confirmDeleteCritical}
  requireCheckbox={confirmDeleteCritical}
  checkboxLabel="I understand this will permanently delete all connections inside this folder"
  on:confirm={handleConfirmDelete}
  on:cancel={() => (confirmDeleteShow = false)}
/>

<ImportPuTTYDialog bind:show={showImportDialog} />
