<script lang="ts">
  import {
    connections,
    creationTargetFolderId,
    detailsConnectionId,
    expandedFolderIds,
    favorites,
    folders,
    selectedConnectionId,
    selectedConnectionIds,
    sessions,
    pingResults,
    type Connection,
    type Folder,
  } from '../stores/appState';
  import {
    createNewFolderInFolder,
    deleteFolders,
    saveFolder,
  } from '../actions/folderActions';
  import {
    createNewConnectionInFolder,
    deleteConnections,
    saveConnection,
  } from '../actions/connectionActions';
  import { openSession } from '../actions/sessionActions';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import ImportPuTTYDialog from './ImportPuTTYDialog.svelte';
  import ImportSSHConfigDialog from './ImportSSHConfigDialog.svelte';
  import ImportMenu from './remoteTree/ImportMenu.svelte';
  import type { MenuAnchorRect } from './clampMenuPosition';
  import RemoteTreeContextMenu from './RemoteTreeContextMenu.svelte';
  import { openContextMenu, releaseContextMenu } from './contextMenuManager';
  import { buildTree, flattenTree } from './remoteTree/buildTree';
  import { describeDeleteTargets } from './remoteTree/deletePrompt';
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
    connectionIdsInSelection,
    deleteTargetCount,
    deleteTargets,
    folderIdsInSelection,
    prepareContextMenuSelection,
    selectionDeleteTargets,
    selectTreeNode,
    shouldClearTreeSelection,
    syncSelectionStores,
    type DeleteTargets,
    type SelectionStores,
  } from './remoteTree/selection';
  import { discoveryNodeId, emptyDragVisualState, type DragPayload, type DragVisualState, type DiscoveryRow, type TreeNode } from './remoteTree/types';
  import { clampMenuPosition } from './clampMenuPosition';
  import { onMount, tick } from 'svelte';
  import { get } from 'svelte/store';
  import {
    openNodeDetails,
    closeNodeDetails,
    nodeDetailsYieldToConnection,
  } from '../stores/nodeDetailsState';
  import { invokeDiscoveryAction } from '../api/discovery';
  import {
    computeDiscoveryMenu,
    defaultDiscoveryAction,
    deleteMenuItem,
    type DiscoveryMenu,
    type DiscoveryMenuItem,
  } from './remoteTree/discoveryActions';
  import {
    isRowSelected,
    moveDiscoverySelection,
    selectDiscoveryRow,
    selectedDiscoveryRows,
  } from './remoteTree/discoverySelection';
  import {
    clearDiscoverySelection,
    discoveryExpanded,
    discoveryIcons,
    discoveryPluginNames,
    discoverySelection,
    discoverySnapshots,
    forgetUnavailableDiscovery,
    reconcileDiscoverySelection,
    refreshDiscoveryIcons,
    setDiscoveryNodeExpanded,
    toggleDiscoveryNode,
    toggleDiscoveryRoot,
  } from '../stores/discoveryState';
  import './remoteTree/remoteTreeShared.css';

  const selectionStores: SelectionStores = {
    selectedConnectionId,
    selectedConnectionIds,
    creationTargetFolderId,
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
  let ctxMenu = {
    show: false,
    x: 0,
    y: 0,
    node: null as TreeNode | null,
    discoveryMenu: null as DiscoveryMenu | null,
  };
  let confirmDeleteShow = false;
  let confirmDeleteTargets: DeleteTargets = { folderIds: [], connectionIds: [] };
  let confirmDeleteTitle = '';
  let confirmDeleteMessage = '';
  let confirmDeleteCritical = false;
  let confirmDeleteCheckboxLabel = '';
  let showImportDialog = false;
  let showSSHConfigDialog = false;
  let importMenu: { show: boolean; anchor: MenuAnchorRect | null } = { show: false, anchor: null };
  let confirmActionShow = false;
  let confirmActionMessage = '';
  let pendingAction: { item: DiscoveryMenuItem; menu: DiscoveryMenu } | null = null;

  onMount(() => {
    // Icons ride along on ListPlugins and are cached in the registry, so this is
    // one call for every icon of every plugin — there is no per-icon endpoint.
    void refreshDiscoveryIcons();
  });

  // Discovery enumerates through a LEADING session, and ADR-014 defines that as
  // the earliest one in `ready` state — so `connecting` and `error` do not count.
  // Offering the expander for those would promise a subtree that cannot exist
  // yet and answer with "No discovered resources".
  $: discoveryAvailableIds = new Set(
    $sessions.filter((s) => s.state === 'ready').map((s) => s.connectionId)
  );
  // Ordered before buildTree deliberately: when the last ready session goes, the
  // backend deletes the tree and the connection row stops drawing its expander,
  // so the expansion has to go in the same pass. buildTree knows nothing about
  // sessions, and an expansion that outlived its session would keep rendering
  // rows with no control left to collapse them.
  $: forgetUnavailableDiscovery(discoveryAvailableIds);
  $: tree = buildTree($folders, $connections, $expandedFolderIds, searchQuery, {
    snapshots: $discoverySnapshots,
    expanded: $discoveryExpanded,
  });
  $: flatNodes = flattenTree(tree);
  $: favoriteConns = $connections.filter((c) => $favorites.has(c.id));
  $: sessionStatusByConnId = buildSessionStatusMap($sessions);
  $: discoveryRows = flatNodes
    .filter((n) => n.type === 'discovery' && n.discovery)
    .map((n) => n.discovery as DiscoveryRow);
  // A row that vanished from a republished snapshot must leave the selection,
  // and an emptied selection closes the action menu.
  $: {
    reconcileDiscoverySelection(discoveryRows);
  }
  $: if ($discoverySelection.keys.size === 0 && ctxMenu.discoveryMenu) closeContextMenu();
  // Search never auto-expands a discovery node: expanding is what publishes an
  // `observe`, so one keystroke would fan observe/publish out across every
  // connection at once. Say so rather than let the user believe the subtree was
  // searched and found nothing.
  $: searchHint =
    searchQuery && $discoveryExpanded.size > 0
      ? 'Search covers connections and folders. Discovered resources are not searched.'
      : '';
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

  /** Single entry point for every delete verb — context menu, row button, Delete key. */
  function requestDelete(targets: DeleteTargets) {
    if (deleteTargetCount(targets) === 0) return;
    const prompt = describeDeleteTargets(targets, $folders, $connections);
    confirmDeleteTargets = targets;
    confirmDeleteTitle = prompt.title;
    confirmDeleteMessage = prompt.message;
    confirmDeleteCritical = prompt.critical;
    confirmDeleteCheckboxLabel = prompt.checkboxLabel;
    confirmDeleteShow = true;
  }

  function showContextMenu(e: MouseEvent, node: TreeNode) {
    e.preventDefault();
    e.stopPropagation();
    if (node.type === 'discovery') {
      if (!node.discovery) return;
      // Right-click on an unselected row solo-selects it, same file-manager rule
      // the connection tree uses — but inside discovery's own selection.
      // isRowSelected, not keys.has: a discoveryKey is (pluginId, nodeId) and is
      // not connection-scoped, so one plugin publishing the same node on two
      // hosts would otherwise make a row look already-selected because its twin
      // under another connection is, and the menu would be built from that other
      // connection's rows.
      if (!isRowSelected($discoverySelection, node.discovery)) {
        selectDiscoveryNode(node.discovery);
      }
      const rows = selectedDiscoveryRows(get(discoverySelection), discoveryRows);
      const menu = computeDiscoveryMenu(rows);
      const pos = clampMenuPosition(
        { left: e.clientX, top: e.clientY, right: e.clientX, bottom: e.clientY },
        260,
        Math.max(60, menu.items.length * 28 + (menu.notice ? 40 : 0))
      );
      ctxMenu = { show: true, x: pos.left, y: pos.top, node, discoveryMenu: menu };
      openContextMenu(closeContextMenu);
      return;
    }
    const prep = prepareContextMenuSelection(node, selectedPaths);
    if (prep) applySelection(prep);
    const pos = clampMenuPosition(
      { left: e.clientX, top: e.clientY, right: e.clientX, bottom: e.clientY },
      200,
      node.type === 'folder' ? 150 : 160
    );
    ctxMenu = { show: true, x: pos.left, y: pos.top, node, discoveryMenu: null };
    openContextMenu(closeContextMenu);
  }

  function closeContextMenu() {
    releaseContextMenu(closeContextMenu);
    ctxMenu = { ...ctxMenu, show: false, discoveryMenu: null };
  }

  /**
   * Moving focus into a discovery row clears the connection/folder selection and
   * vice versa. Two independent sets, only one of them ever populated — that is
   * what stops a Shift-drag through a subtree from ending up aimed at Delete.
   */
  function selectDiscoveryNode(row: DiscoveryRow, e?: MouseEvent) {
    if (selectedPaths.size > 0) {
      selectedPaths = clearTreeSelection(selectionStores);
      lastSelectedPath = null;
    }
    discoverySelection.update((sel) => selectDiscoveryRow(sel, row, discoveryRows, e));
    // The details panel follows the selection, and only for a single row: a panel showing the
    // properties of one of four selected nodes would be lying about which one (ADR-015 §3).
    const selected = get(discoverySelection);
    if (selected.keys.size === 1) {
      openNodeDetails({
        connectionId: row.connectionId,
        pluginId: row.pluginId,
        nodeId: row.nodeId,
        label: row.label,
      });
    } else {
      closeNodeDetails();
    }
  }

  async function runDiscoveryAction(item: DiscoveryMenuItem, menu: DiscoveryMenu) {
    if (item.disabled || menu.nodeIds.length === 0) return;
    await invokeDiscoveryAction(menu.connectionId, menu.pluginId, menu.nodeIds, item.id);
  }

  function requestDiscoveryAction(item: DiscoveryMenuItem, menu: DiscoveryMenu) {
    closeContextMenu();
    if (item.confirm) {
      // A mass action names the count: "Remove" over 40 containers is a very
      // different decision from "Remove" over one.
      confirmActionMessage =
        menu.nodeIds.length > 1
          ? `${item.confirm} (${menu.nodeIds.length} items)`
          : item.confirm;
      pendingAction = { item, menu };
      confirmActionShow = true;
      return;
    }
    void runDiscoveryAction(item, menu);
  }

  async function handleConfirmDiscoveryAction() {
    confirmActionShow = false;
    const pending = pendingAction;
    pendingAction = null;
    if (pending) await runDiscoveryAction(pending.item, pending.menu);
  }

  /**
   * Enter on a discovery row. It acts on the SELECTION, not on whichever row
   * happens to hold focus, so that what is highlighted and what the key does are
   * the same thing. defaultActionId is a single-row affordance, so a
   * multi-selection deliberately resolves to nothing rather than silently
   * running one node's default across a set the user never named.
   */
  function activateDiscoveryRow(row: DiscoveryRow) {
    const selected = selectedDiscoveryRows(get(discoverySelection), discoveryRows);
    // Membership is (connectionId, key), never key alone — otherwise Enter on a
    // row under one connection could run over a selection under another that
    // happens to hold the same plugin's node of the same name.
    const target = isRowSelected(get(discoverySelection), row) ? selected : [row];
    const item = defaultDiscoveryAction(target);
    if (item) requestDiscoveryAction(item, computeDiscoveryMenu(target));
    else if (target.length === 1 && row.kind === 'group') {
      void toggleDiscoveryNode(row.connectionId, row.key);
    }
  }

  /**
   * Keeps DOM focus on the row the selection just moved to.
   *
   * Without this, arrow keys would move the highlight while Enter and the
   * left/right arrows kept reading the previously focused row — the user would
   * see one row selected and the keyboard would act on another.
   */
  async function focusDiscoveryRow(row: DiscoveryRow | undefined) {
    if (!row || typeof document === 'undefined') return;
    await tick();
    // Addressed by TreeNode.id, which is scoped by (connectionId, pluginId,
    // nodeId) — NOT by discoveryKey, which omits the connection. Two connections
    // showing the same plugin's `containers` produce two rows with the same key,
    // and querySelector would return whichever came first in the document, so
    // focus could land in a different connection's subtree from the one the
    // selection just moved through.
    const id = discoveryNodeId(row.connectionId, row.pluginId, row.nodeId);
    const escaped = typeof CSS !== 'undefined' && CSS.escape ? CSS.escape(id) : id.replace(/["\\]/g, '\\$&');
    const el = document.querySelector<HTMLElement>(
      `.remote-tree .tree-node[data-discovery-id="${escaped}"]`
    );
    el?.focus();
  }

  function handleWindowClick(e: MouseEvent) {
    closeContextMenu();
    importMenu = { ...importMenu, show: false };
    const target = e.target as HTMLElement | null;
    if (!shouldClearTreeSelection(target)) return;
    clearDiscoverySelection();
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
    const node = ctxMenu.node;
    closeContextMenu();
    requestDelete(deleteTargets(node.id, selectedPaths, $connections, $folders));
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
    const { folderIds, connectionIds } = confirmDeleteTargets;
    // Folders first: their cascade takes nested connections with it, so the
    // connection batch that follows only names rows that outlived it.
    await deleteFolders(folderIds);
    await deleteConnections(connectionIds);
    // Rebuilt from the refreshed stores rather than from the requested ids: a
    // deleted folder also took away rows nobody named explicitly.
    const alive = new Set([...$connections.map((c) => c.id), ...$folders.map((f) => f.id)]);
    selectedPaths = new Set([...selectedPaths].filter((id) => alive.has(id)));
    syncSelectionStores(selectedPaths, $connections, $folders, selectionStores);
    if (!alive.has($selectedConnectionId)) selectedConnectionId.set('');
    if (!alive.has($detailsConnectionId)) detailsConnectionId.set('');
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
    // null zone = a discovery row: no indicator is drawn at all, so a drag
    // crossing an expanded subtree never suggests an insertion that cannot
    // happen.
    const zone = computeDropZone(e, node);
    if (zone === null || !shouldShowDropIndicator(dragPayload, node, zone, $connections, $folders, flatNodes)) {
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
    if (zone === null) {
      clearDragState();
      return;
    }
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
    // Discovery first, and exclusively: the two selections are disjoint by
    // construction (a discovery row never enters selectedPaths — see
    // remoteTree/selection.ts), so a non-empty discovery selection means the key
    // was pressed with plugin nodes highlighted and nothing else.
    const discoveryRowsSelected = selectedDiscoveryRows(get(discoverySelection), discoveryRows);
    if (discoveryRowsSelected.length > 0) {
      const menu = computeDiscoveryMenu(discoveryRowsSelected);
      const item = deleteMenuItem(menu);
      // No marked action is a silent no-op on purpose. The core does not know
      // which of a plugin's actions removes anything, and inventing one from a
      // label is how a keystroke deletes something nobody offered to delete.
      if (item) {
        ev.preventDefault();
        requestDiscoveryAction(item, menu);
      }
      return;
    }
    const targets = selectionDeleteTargets(selectedPaths, $connections, $folders);
    if (deleteTargetCount(targets) > 0) {
      ev.preventDefault();
      requestDelete(targets);
    }
  }

  function handleSelectNode(id: string, e?: MouseEvent) {
    clearDiscoverySelection();
    // The sidebar has one details slot: selecting a connection or folder closes any open node
    // panel, exactly as selecting a node clears the connection selection (ADR-015 §3).
    nodeDetailsYieldToConnection();
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
    const { event, node } = e.detail;
    if (node.type === 'discovery') {
      if (node.discovery) selectDiscoveryNode(node.discovery, event);
      return;
    }
    handleSelectNode(node.id, event);
  }

  function handleNodeDblclick(e: CustomEvent<{ node: TreeNode }>) {
    const { node } = e.detail;
    if (node.type === 'discovery') {
      if (node.discovery) activateDiscoveryRow(node.discovery);
      return;
    }
    if (node.connection) openSession(node.connection.id);
    else toggleFolder(node.id);
  }

  function handleNodeKeydown(e: CustomEvent<{ event: KeyboardEvent; node: TreeNode }>) {
    const { event, node } = e.detail;
    if (node.type === 'discovery') {
      const row = node.discovery;
      if (!row) return;
      // Arrows walk the subtree the same way they walk folders; Enter runs the
      // node's defaultActionId, which is the plugin's idea of "the obvious
      // thing", not the core's.
      if (event.key === 'Enter') {
        event.preventDefault();
        activateDiscoveryRow(row);
      } else if (event.key === 'ArrowRight' && row.kind === 'group' && !row.expanded) {
        event.preventDefault();
        void setDiscoveryNodeExpanded(row.connectionId, row.key, true);
      } else if (event.key === 'ArrowLeft' && row.kind === 'group' && row.expanded) {
        event.preventDefault();
        void setDiscoveryNodeExpanded(row.connectionId, row.key, false);
      } else if (event.key === 'ArrowDown' || event.key === 'ArrowUp') {
        // Shift extends, but only across siblings — the invariant lives in
        // moveDiscoverySelection, not here.
        event.preventDefault();
        const direction = event.key === 'ArrowDown' ? 1 : -1;
        discoverySelection.update((sel) =>
          moveDiscoverySelection(sel, discoveryRows, direction, event.shiftKey)
        );
        // Focus follows the selection so Enter and the left/right arrows keep
        // acting on the row the user can see is current. Resolved through the
        // selection's own connectionId, so it can only ever land in the subtree
        // the selection lives in.
        const moved = get(discoverySelection);
        void focusDiscoveryRow(
          discoveryRows.find((r) => r.connectionId === moved.connectionId && r.key === moved.lastKey)
        );
      }
      return;
    }
    if (event.key === 'Enter' && node.connection) openSession(node.connection.id);
    if (event.key === 'Enter' && node.type === 'folder') toggleFolder(node.id);
  }

  function handleDeleteConnection(e: CustomEvent<{ connection?: Connection; multi?: boolean }>) {
    if (!e.detail.connection) return;
    if (e.detail.multi) requestDelete(selectionDeleteTargets(selectedPaths, $connections, $folders));
    else requestDelete({ folderIds: [], connectionIds: [e.detail.connection.id] });
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
    onNewConnection={() => createNewConnectionInFolder($creationTargetFolderId)}
    onNewFolder={() => createNewFolderInFolder($creationTargetFolderId)}
    onImport={(anchor) => (importMenu = { show: true, anchor })}
    importMenuOpen={importMenu.show}
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
    {discoveryAvailableIds}
    discoveryIcons={$discoveryIcons}
    discoveryPluginNames={$discoveryPluginNames}
    discoverySelection={$discoverySelection}
    {searchHint}
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
    on:deleteFolder={({ detail }) =>
      detail.folder && requestDelete(deleteTargets(detail.folder.id, selectedPaths, $connections, $folders))}
    on:startRenameConnection={({ detail }) => detail.connection && startRenameConnection(detail.connection)}
    on:deleteConnection={handleDeleteConnection}
    on:toggleDiscoveryRoot={({ detail }) => toggleDiscoveryRoot(detail.connectionId)}
    on:toggleDiscoveryNode={({ detail }) => toggleDiscoveryNode(detail.row.connectionId, detail.row.key)}
  />
</div>

<RemoteTreeContextMenu
  x={ctxMenu.x}
  y={ctxMenu.y}
  show={ctxMenu.show}
  isFolder={ctxMenu.node?.type === 'folder'}
  isConnection={ctxMenu.node?.type === 'connection'}
  isFavorite={ctxMenu.node?.type === 'connection' && ctxMenu.node?.id ? $favorites.has(ctxMenu.node.id) : false}
  discoveryMenu={ctxMenu.discoveryMenu}
  on:delete={handleCtxDelete}
  on:edit={handleCtxEdit}
  on:newConnection={handleCtxNewConnection}
  on:newFolder={handleCtxNewFolder}
  on:toggleFavorite={() => ctxMenu.node?.type === 'connection' && toggleFavorite(ctxMenu.node.id)}
  on:invokeAction={({ detail }) => ctxMenu.discoveryMenu && requestDiscoveryAction(detail, ctxMenu.discoveryMenu)}
/>

<ConfirmDialog
  show={confirmActionShow}
  title="Confirm action"
  message={confirmActionMessage}
  critical={!!pendingAction?.item.danger}
  on:confirm={handleConfirmDiscoveryAction}
  on:cancel={() => {
    confirmActionShow = false;
    pendingAction = null;
  }}
/>

<ConfirmDialog
  show={confirmDeleteShow}
  title={confirmDeleteTitle}
  message={confirmDeleteMessage}
  critical={confirmDeleteCritical}
  requireCheckbox={confirmDeleteCritical}
  checkboxLabel={confirmDeleteCheckboxLabel}
  on:confirm={handleConfirmDelete}
  on:cancel={() => (confirmDeleteShow = false)}
/>

<ImportMenu
  show={importMenu.show}
  anchor={importMenu.anchor}
  on:close={() => (importMenu = { ...importMenu, show: false })}
  on:select={({ detail }) => {
    if (detail === 'putty') showImportDialog = true;
    else showSSHConfigDialog = true;
  }}
/>

<ImportPuTTYDialog bind:show={showImportDialog} />
<ImportSSHConfigDialog bind:show={showSSHConfigDialog} />
