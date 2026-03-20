<script lang="ts">
  import { folders, connections, sessions, selectedConnectionId, selectedConnectionIds, selectedFolderId, expandedFolderIds, pingResults, favorites, type Folder, type Connection } from '../stores/appState';
  import { openSession, moveConnections, moveFolder, reorderConnections, reorderFolders, saveFolder, saveConnection, deleteFolder, deleteConnection, createNewConnectionInFolder, createNewFolderInFolder } from '../stores/api';
  import { Folder as FolderIcon, FolderOpen, Monitor, MonitorPlay, Terminal, Cable, Globe, Pencil, X, Plus, FolderPlus, MonitorDot, ChevronRight, ChevronDown, ChevronsDownUp, ChevronsUpDown, Search, CheckCircle2, Loader2, XCircle, Circle, Download } from 'lucide-svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import ImportPuTTYDialog from './ImportPuTTYDialog.svelte';
  import RemoteTreeContextMenu from './RemoteTreeContextMenu.svelte';

  function pingColor(pingMap: Map<string, any>, connId: string): string {
    const r = pingMap.get(connId);
    if (!r) return 'transparent';
    if (!r.reachable) return 'var(--danger, #f44747)';
    if (r.latencyMs < 100) return '#4caf50';
    if (r.latencyMs < 300) return '#ffb300';
    if (r.latencyMs < 1000) return '#ff6f00';
    return 'var(--danger, #f44747)';
  }

  function protocolIcon(protocol?: string) {
    switch (protocol || 'ssh') {
      case 'rdp': return MonitorPlay;
      case 'telnet': return Terminal;
      case 'serial': return Cable;
      case 'http': return Globe;
      default: return Monitor;
    }
  }

  function pingTooltip(pingMap: Map<string, any>, connId: string): string {
    const r = pingMap.get(connId);
    if (!r) return 'Not pinged yet';
    if (!r.reachable) return 'Unreachable';
    return `${r.latencyMs}ms`;
  }

  type ConnectionStatus = 'active' | 'connecting' | 'error' | 'disconnected';
  $: sessionStatusByConnId = (() => {
    const m = new Map<string, ConnectionStatus>();
    for (const s of $sessions) {
      const st: ConnectionStatus = s.state === 'ready' ? 'active' : s.state === 'connecting' ? 'connecting' : s.state === 'error' ? 'error' : 'disconnected';
      m.set(s.connectionId, st);
    }
    return m;
  })();

  let editingFolderId: string | null = null;
  let editingFolderName = '';
  let editingConnId: string | null = null;
  let editingConnName = '';
  let dragOverId: string | null = null;
  let dragOverRoot = false;
  let dragOverDropZone: 'folder' | 'before' | 'after' | null = null;
  let dragOverTargetId: string | null = null;
  let dragPayload: { type: 'folder' | 'connection'; id: string } | null = null;

  let searchQuery = '';

  let ctxMenu = { show: false, x: 0, y: 0, node: null as TreeNode | null };

  let confirmDeleteShow = false;
  let confirmDeleteType: 'connection' | 'folder' = 'connection';
  let confirmDeleteId = '';
  let confirmDeleteIds: string[] = [];
  let confirmDeleteName = '';
  let confirmDeleteCritical = false;
  let confirmDeleteChildCount = 0;
  let showImportDialog = false;

  interface TreeNode {
    type: 'folder' | 'connection';
    id: string;
    name: string;
    depth: number;
    parentId: string;
    folder?: Folder;
    connection?: Connection;
    children?: TreeNode[];
    expanded?: boolean;
    tags?: string[];
  }

  function matchesSearch(q: string, name: string, host?: string): boolean {
    const lower = q.toLowerCase();
    if (name.toLowerCase().includes(lower)) return true;
    if (host && host.toLowerCase().includes(lower)) return true;
    return false;
  }

  function buildTree(folderList: Folder[], connList: Connection[], expanded: Set<string>, query: string): TreeNode[] {
    const folderMap = new Map<string, Folder[]>();
    const connMap = new Map<string, Connection[]>();

    for (const f of folderList) {
      const pid = f.parentId || '';
      if (!folderMap.has(pid)) folderMap.set(pid, []);
      folderMap.get(pid)!.push(f);
    }

    for (const c of connList) {
      const fid = c.folderId || '';
      if (!connMap.has(fid)) connMap.set(fid, []);
      connMap.get(fid)!.push(c);
    }

    function hasMatchInSubtree(parentId: string): boolean {
      for (const c of connMap.get(parentId) || []) {
        if (matchesSearch(query, c.name, c.host)) return true;
      }
      for (const f of folderMap.get(parentId) || []) {
        if (matchesSearch(query, f.name)) return true;
        if (hasMatchInSubtree(f.id)) return true;
      }
      return false;
    }

    function buildLevel(parentId: string, depth: number): TreeNode[] {
      const nodes: TreeNode[] = [];
      const subFolders = (folderMap.get(parentId) || []).sort((a, b) => a.order - b.order);
      for (const f of subFolders) {
        const childMatches = hasMatchInSubtree(f.id);
        const selfMatches = matchesSearch(query, f.name);
        const showFolder = !query || selfMatches || childMatches;
        if (!showFolder) continue;

        const isExpanded = expanded.has(f.id) || (query && childMatches);
        const node: TreeNode = {
          type: 'folder',
          id: f.id,
          name: f.name,
          depth,
          parentId,
          folder: f,
          expanded: isExpanded,
          children: isExpanded ? buildLevel(f.id, depth + 1) : [],
        };
        nodes.push(node);
      }
      const subConns = (connMap.get(parentId) || []).sort((a, b) => a.order - b.order);
      for (const c of subConns) {
        if (query && !matchesSearch(query, c.name, c.host)) continue;
        nodes.push({
          type: 'connection',
          id: c.id,
          name: c.name,
          depth,
          parentId,
          connection: c,
          tags: c.tags || [],
        });
      }
      return nodes;
    }

    return buildLevel('', 0);
  }

  $: tree = buildTree($folders, $connections, $expandedFolderIds, searchQuery);

  function toggleFolder(id: string) {
    expandedFolderIds.update(set => {
      const next = new Set(set);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }

  let lastClickedConnectionId: string | null = null;

  function selectConnection(id: string, e?: MouseEvent) {
    const ctrl = e?.ctrlKey ?? false;
    const shift = e?.shiftKey ?? false;
    selectedFolderId.set('');
    if (ctrl) {
      selectedConnectionIds.update(set => {
        const next = new Set(set);
        if (next.has(id)) next.delete(id);
        else next.add(id);
        if (next.size > 0) lastClickedConnectionId = id;
        return next;
      });
      selectedConnectionId.set(id);
    } else if (shift && lastClickedConnectionId) {
      const connNodes = flatNodes.filter(n => n.type === 'connection');
      const idxLast = connNodes.findIndex(n => n.id === lastClickedConnectionId);
      const idxCur = connNodes.findIndex(n => n.id === id);
      if (idxLast >= 0 && idxCur >= 0) {
        const [lo, hi] = idxLast < idxCur ? [idxLast, idxCur] : [idxCur, idxLast];
        const ids = connNodes.slice(lo, hi + 1).map(n => n.id);
        selectedConnectionIds.set(new Set(ids));
        selectedConnectionId.set(id);
      } else {
        selectedConnectionIds.set(new Set([id]));
        selectedConnectionId.set(id);
        lastClickedConnectionId = id;
      }
    } else {
      selectedConnectionIds.set(new Set([id]));
      selectedConnectionId.set(id);
      lastClickedConnectionId = id;
    }
  }

  function selectFolder(id: string) {
    selectedFolderId.set(id);
    selectedConnectionId.set('');
    selectedConnectionIds.set(new Set());
    lastClickedConnectionId = null;
  }

  function handleDblClick(conn: Connection) {
    openSession(conn.id);
  }

  // --- DnD ---
  function handleDragStart(e: DragEvent, node: TreeNode) {
    if (!e.dataTransfer) return;
    e.dataTransfer.effectAllowed = 'move';
    dragPayload = { type: node.type, id: node.id };
    e.dataTransfer.setData('application/json', JSON.stringify(dragPayload));
  }

  function handleDragOver(e: DragEvent) {
    e.preventDefault();
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
  }

  function handleFolderDragEnter(folderId: string) {
    dragOverId = folderId;
    dragOverRoot = false;
  }

  function handleNodeDragOver(e: DragEvent, node: TreeNode) {
    e.preventDefault();
    e.stopPropagation();
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'move';
    if (!dragPayload || dragPayload.id === node.id) return;
    const rect = (e.currentTarget as HTMLElement).getBoundingClientRect();
    const y = e.clientY - rect.top;
    const ratio = y / rect.height;
    if (node.type === 'folder') {
      if (ratio < 0.25) {
        dragOverDropZone = 'before';
        dragOverTargetId = node.id;
        dragOverId = null;
      } else if (ratio > 0.75) {
        dragOverDropZone = 'after';
        dragOverTargetId = node.id;
        dragOverId = null;
      } else {
        dragOverDropZone = 'folder';
        dragOverId = node.id;
        dragOverTargetId = node.id;
      }
    } else {
      dragOverDropZone = ratio < 0.5 ? 'before' : 'after';
      dragOverTargetId = node.id;
      dragOverId = null;
    }
    dragOverRoot = false;
  }

  function handleFolderDragLeave() {
    dragOverId = null;
    dragOverDropZone = null;
    dragOverTargetId = null;
  }

  function handleRootDragEnter() {
    dragOverRoot = true;
    dragOverId = null;
  }

  function handleRootDragLeave(e: DragEvent) {
    const rt = e.relatedTarget as HTMLElement | null;
    if (!rt || !e.currentTarget || !(e.currentTarget as HTMLElement).contains(rt)) {
      dragOverRoot = false;
      dragOverDropZone = null;
      dragOverTargetId = null;
    }
  }

  function clearDragState() {
    dragOverId = null;
    dragOverRoot = false;
    dragOverDropZone = null;
    dragOverTargetId = null;
    dragPayload = null;
  }

  async function handleDropBetween(e: DragEvent, targetNode: TreeNode, position: 'before' | 'after') {
    e.preventDefault();
    e.stopPropagation();
    if (!e.dataTransfer || !dragPayload) return;
    try {
      const siblings = flatNodes.filter(n => n.parentId === targetNode.parentId && n.depth === targetNode.depth && n.type === dragPayload!.type);
      const targetIdx = siblings.findIndex(n => n.id === targetNode.id);
      if (targetIdx < 0) return;
      const draggedIdx = siblings.findIndex(n => n.id === dragPayload!.id);
      if (draggedIdx < 0) return;
      const reordered = [...siblings];
      const [removed] = reordered.splice(draggedIdx, 1);
      const newIdx = reordered.findIndex(n => n.id === targetNode.id);
      const finalIdx = position === 'before' ? newIdx : newIdx + 1;
      reordered.splice(finalIdx, 0, removed);
      const ids = reordered.map(n => n.id);
      if (dragPayload.type === 'connection') {
        await reorderConnections(ids, targetNode.parentId);
      } else {
        await reorderFolders(ids, targetNode.parentId);
      }
    } catch (err) {
      // errors are handled by api.ts -> errorStore
    }
    clearDragState();
  }

  async function handleDropOnFolder(e: DragEvent, targetFolderId: string) {
    e.preventDefault();
    e.stopPropagation();
    if (!e.dataTransfer) return;
    try {
      const payload = JSON.parse(e.dataTransfer.getData('application/json'));
      if (payload.type === 'connection') {
        await moveConnections([payload.id], targetFolderId);
      } else if (payload.type === 'folder' && payload.id !== targetFolderId) {
        await moveFolder(payload.id, targetFolderId);
      }
    } catch (err) {
      // errors are handled by api.ts -> errorStore
    }
    clearDragState();
  }

  async function handleDropOnRoot(e: DragEvent) {
    e.preventDefault();
    e.stopPropagation();
    if (!e.dataTransfer) return;
    try {
      const payload = JSON.parse(e.dataTransfer.getData('application/json'));
      if (payload.type === 'connection') {
        await moveConnections([payload.id], '');
      } else if (payload.type === 'folder') {
        await moveFolder(payload.id, '');
      }
    } catch (err) {
      // errors are handled by api.ts -> errorStore
    }
    clearDragState();
  }

  // --- Folder CRUD ---
  function startRenameFolder(f: Folder) {
    editingFolderId = f.id;
    editingFolderName = f.name;
  }

  async function confirmRename() {
    if (!editingFolderName.trim() || !editingFolderId) { editingFolderId = null; return; }
    const f = $folders.find(x => x.id === editingFolderId);
    if (f) await saveFolder({ ...f, name: editingFolderName.trim() });
    editingFolderId = null;
  }

  // --- Connection rename ---
  function startRenameConnection(c: Connection) {
    editingConnId = c.id;
    editingConnName = c.name;
  }

  async function confirmRenameConnection() {
    if (!editingConnName.trim() || !editingConnId) { editingConnId = null; return; }
    const c = $connections.find(x => x.id === editingConnId);
    if (c) await saveConnection({ ...c, name: editingConnName.trim() });
    editingConnId = null;
  }

  // --- Delete with confirmation ---
  function countConnectionsInFolder(folderId: string): number {
    let count = $connections.filter(c => c.folderId === folderId).length;
    const children = $folders.filter(f => f.parentId === folderId);
    for (const child of children) count += countConnectionsInFolder(child.id);
    return count;
  }

  function requestDeleteConnection(c: Connection) {
    requestDeleteConnections([c.id]);
  }

  function requestDeleteConnections(ids: string[]) {
    confirmDeleteType = 'connection';
    confirmDeleteIds = ids;
    confirmDeleteId = ids[0] || '';
    confirmDeleteName = ids.length === 1 ? ($connections.find(x => x.id === ids[0])?.name ?? '') : '';
    confirmDeleteCritical = ids.length > 1;
    confirmDeleteChildCount = ids.length;
    confirmDeleteShow = true;
  }

  function requestDeleteFolder(f: Folder) {
    const childCount = countConnectionsInFolder(f.id);
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
    ctxMenu = { show: true, x: e.clientX, y: e.clientY, node };
  }

  function closeContextMenu() {
    ctxMenu = { ...ctxMenu, show: false };
  }

  function toggleFavorite(connId: string) {
    favorites.update(s => {
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
    if (ctxMenu.node.type === 'connection' && ctxMenu.node.connection) {
      const ids = $selectedConnectionIds.has(ctxMenu.node.id) ? [...$selectedConnectionIds] : [ctxMenu.node.id];
      requestDeleteConnections(ids);
    } else if (ctxMenu.node.type === 'folder' && ctxMenu.node.folder) {
      requestDeleteFolder(ctxMenu.node.folder);
    }
  }

  async function handleConfirmDelete() {
    confirmDeleteShow = false;
    if (confirmDeleteType === 'connection') {
      for (const id of confirmDeleteIds) {
        await deleteConnection(id);
      }
      selectedConnectionIds.set(new Set());
      if (confirmDeleteIds.includes($selectedConnectionId)) selectedConnectionId.set('');
    } else {
      await deleteFolder(confirmDeleteId);
      if ($selectedFolderId === confirmDeleteId) selectedFolderId.set('');
    }
  }

  function handleTreeKeydown(e: KeyboardEvent) {
    if (e.key === 'Delete' || e.key === 'Backspace') {
      const ids = [...$selectedConnectionIds];
      if (ids.length > 0) {
        e.preventDefault();
        requestDeleteConnections(ids);
      }
    }
  }

  // --- Expand / Collapse All ---
  function expandAll() {
    expandedFolderIds.set(new Set($folders.map(f => f.id)));
  }

  function collapseAll() {
    expandedFolderIds.set(new Set());
  }

  // --- Tags ---
  function tagColor(tag: string): string {
    let hash = 0;
    for (let i = 0; i < tag.length; i++) {
      hash = tag.charCodeAt(i) + ((hash << 5) - hash);
    }
    const h = Math.abs(hash) % 360;
    return `hsl(${h}, 50%, 40%)`;
  }

  function flattenTree(nodes: TreeNode[]): TreeNode[] {
    const result: TreeNode[] = [];
    for (const n of nodes) {
      result.push(n);
      if (n.type === 'folder' && n.expanded && n.children) {
        result.push(...flattenTree(n.children));
      }
    }
    return result;
  }

  $: flatNodes = flattenTree(tree);
  $: favoriteConns = $connections.filter(c => $favorites.has(c.id));
</script>

<svelte:window on:click={closeContextMenu} />
<div
  class="remote-tree"
  class:drag-over-root={dragOverRoot}
  role="tree"
  tabindex="0"
  on:dragover={handleDragOver}
  on:drop={handleDropOnRoot}
  on:dragenter={handleRootDragEnter}
  on:dragleave={handleRootDragLeave}
  on:keydown={handleTreeKeydown}
>
  <div class="search-bar">
    <Search size={12} class="search-icon" />
    <input
      type="text"
      class="search-input"
      placeholder="Search connections..."
      bind:value={searchQuery}
    />
  </div>
  <div class="tree-toolbar">
    <button class="toolbar-btn" on:click={() => createNewConnectionInFolder($selectedFolderId)} title="New Connection">
      <MonitorDot size={14} />
    </button>
    <button class="toolbar-btn" on:click={() => createNewFolderInFolder($selectedFolderId)} title="New Folder">
      <FolderPlus size={14} />
    </button>
    <button class="toolbar-btn" on:click={() => showImportDialog = true} title="Import from PuTTY (.ppk, .reg)">
      <Download size={14} />
    </button>
    <div class="toolbar-spacer"></div>
    <button class="toolbar-btn" on:click={expandAll} title="Expand All">
      <ChevronsUpDown size={14} />
    </button>
    <button class="toolbar-btn" on:click={collapseAll} title="Collapse All">
      <ChevronsDownUp size={14} />
    </button>
  </div>

  <div class="tree-body">
    {#if favoriteConns.length > 0}
      <div class="favorites-section">
        <div class="favorites-header">Favorites</div>
        {#each favoriteConns as conn (conn.id)}
          <div
            class="tree-node connection favorite-node"
            class:selected={$selectedConnectionIds.has(conn.id)}
            style="padding-left: 8px"
            role="treeitem"
            tabindex="0"
            on:click={(e) => selectConnection(conn.id, e)}
            on:dblclick={() => handleDblClick(conn)}
            on:contextmenu={(e) => showContextMenu(e, { type: 'connection', id: conn.id, name: conn.name, depth: 0, connection: conn })}
            on:keydown={(e) => e.key === 'Enter' && handleDblClick(conn)}
          >
            <span class="ping-dot" style="background: {pingColor($pingResults, conn.id)}" title={pingTooltip($pingResults, conn.id)}></span>
            <span class="conn-icon"><svelte:component this={protocolIcon(conn.protocol)} size={14} /></span>
            {#if sessionStatusByConnId.get(conn.id)}
              {@const status = sessionStatusByConnId.get(conn.id) ?? 'disconnected'}
              <span class="conn-status" class:active={status === 'active'} class:connecting={status === 'connecting'} class:error={status === 'error'} title={status}>
                {#if status === 'active'}<CheckCircle2 size={10} />
                {:else if status === 'connecting'}<span class="spinning"><Loader2 size={10} /></span>
                {:else if status === 'error'}<XCircle size={10} />
                {:else}<Circle size={10} />{/if}
              </span>
            {/if}
            <span class="node-name">{conn.name}</span>
            <span class="protocol-badge" title="Protocol">{(conn.protocol || 'ssh').toUpperCase()}</span>
          </div>
        {/each}
      </div>
    {/if}
    {#each flatNodes as node (node.type + '-' + node.id)}
      <div
        class="tree-node"
        class:folder={node.type === 'folder'}
        class:connection={node.type === 'connection'}
        class:selected={
          (node.type === 'connection' && node.id === $selectedConnectionId) ||
          (node.type === 'folder' && node.id === $selectedFolderId)
        }
        class:drop-target={dragOverDropZone === 'folder' && dragOverTargetId === node.id}
        class:drop-target-before={dragOverDropZone === 'before' && dragOverTargetId === node.id}
        class:drop-target-after={dragOverDropZone === 'after' && dragOverTargetId === node.id}
        style="padding-left: {Math.min(8 + node.depth * 12, 100)}px"
        draggable="true"
        role="treeitem"
        aria-selected={
          (node.type === 'connection' && $selectedConnectionIds.has(node.id)) ||
          (node.type === 'folder' && node.id === $selectedFolderId)
        }
        tabindex="0"
        on:dragstart={(e) => handleDragStart(e, node)}
        on:dragend={clearDragState}
        on:dragover={(e) => handleNodeDragOver(e, node)}
        on:dragenter={node.type === 'folder' ? () => handleFolderDragEnter(node.id) : undefined}
        on:dragleave={node.type === 'folder' ? handleFolderDragLeave : undefined}
        on:drop={(e) => {
          if (dragOverDropZone === 'folder' && node.type === 'folder') {
            handleDropOnFolder(e, node.id);
          } else if (dragOverDropZone === 'before' || dragOverDropZone === 'after') {
            handleDropBetween(e, node, dragOverDropZone);
          }
        }}
        on:click={(e) => node.type === 'connection' ? selectConnection(node.id, e) : selectFolder(node.id)}
        on:dblclick={() => node.connection ? handleDblClick(node.connection) : toggleFolder(node.id)}
        on:contextmenu={(e) => showContextMenu(e, node)}
        on:keydown={(e) => {
          if (e.key === 'Enter' && node.connection) handleDblClick(node.connection);
          if (e.key === 'Enter' && node.type === 'folder') toggleFolder(node.id);
        }}
      >
        {#if node.type === 'folder'}
          <span class="folder-arrow" role="button" tabindex="-1" on:click|stopPropagation={() => toggleFolder(node.id)} on:keydown|stopPropagation={(e) => e.key === 'Enter' && toggleFolder(node.id)}>
            {#if node.expanded}<ChevronDown size={12} />{:else}<ChevronRight size={12} />{/if}
          </span>
          {#if editingFolderId === node.id}
            <input
              class="inline-input"
              bind:value={editingFolderName}
              on:blur={confirmRename}
              on:keydown={(e) => { if (e.key === 'Enter') confirmRename(); if (e.key === 'Escape') editingFolderId = null; }}
            />
          {:else}
            <span class="folder-icon">
              {#if node.expanded}<FolderOpen size={14} />{:else}<FolderIcon size={14} />{/if}
            </span>
            <span class="node-name">{node.name}</span>
            <div class="folder-actions">
              <button class="micro-btn" on:click|stopPropagation={() => createNewFolderInFolder(node.id)} title="New subfolder"><Plus size={12} /></button>
              <button class="micro-btn" on:click|stopPropagation={() => startRenameFolder(node.folder)} title="Rename"><Pencil size={12} /></button>
              <button class="micro-btn danger" on:click|stopPropagation={() => requestDeleteFolder(node.folder)} title="Delete"><X size={12} /></button>
            </div>
          {/if}
        {:else}
          <span class="ping-dot" style="background: {pingColor($pingResults, node.id)}" title={pingTooltip($pingResults, node.id)}></span>
          <span class="conn-icon"><svelte:component this={protocolIcon(node.connection?.protocol)} size={14} /></span>
          {#if sessionStatusByConnId.get(node.id)}
            {@const status = sessionStatusByConnId.get(node.id) ?? 'disconnected'}
            <span class="conn-status" class:active={status === 'active'} class:connecting={status === 'connecting'} class:error={status === 'error'} title={status}>
              {#if status === 'active'}<CheckCircle2 size={10} />
              {:else if status === 'connecting'}<span class="spinning"><Loader2 size={10} /></span>
              {:else if status === 'error'}<XCircle size={10} />
              {:else}<Circle size={10} />{/if}
            </span>
          {/if}
          {#if editingConnId === node.id}
            <input
              class="inline-input"
              bind:value={editingConnName}
              on:blur={confirmRenameConnection}
              on:keydown={(e) => { if (e.key === 'Enter') confirmRenameConnection(); if (e.key === 'Escape') editingConnId = null; }}
            />
          {:else}
            <span class="node-name">{node.name}</span>
            {#if node.tags && node.tags.length > 0}
              <span class="tag-chips" title={node.tags.join(', ')}>
                {#each node.tags.slice(0, 2) as tag}
                  <span class="tag-chip" style="background: {tagColor(tag)}">{tag}</span>
                {/each}
                {#if node.tags.length > 2}
                  <span class="tag-more">+{node.tags.length - 2}</span>
                {/if}
              </span>
            {/if}
            <span class="protocol-badge" title="Protocol">{(node.connection?.protocol || 'ssh').toUpperCase()}</span>
            <div class="conn-actions">
              <button class="micro-btn" on:click|stopPropagation={() => startRenameConnection(node.connection)} title="Rename"><Pencil size={12} /></button>
              <button class="micro-btn danger" on:click|stopPropagation={() => ($selectedConnectionIds.has(node.id) && $selectedConnectionIds.size > 1) ? requestDeleteConnections([...$selectedConnectionIds]) : requestDeleteConnection(node.connection)} title="Delete"><X size={12} /></button>
            </div>
          {/if}
        {/if}
      </div>
    {/each}

    {#if flatNodes.length === 0}
      <div class="empty-tree">No connections yet</div>
    {/if}
  </div>
</div>

<RemoteTreeContextMenu
  x={ctxMenu.x}
  y={ctxMenu.y}
  show={ctxMenu.show}
  isConnection={ctxMenu.node?.type === 'connection'}
  isFavorite={ctxMenu.node?.type === 'connection' && ctxMenu.node?.id ? $favorites.has(ctxMenu.node.id) : false}
  on:delete={handleCtxDelete}
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
  on:cancel={() => confirmDeleteShow = false}
/>

<ImportPuTTYDialog bind:show={showImportDialog} />

<style>
  .remote-tree {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    user-select: none;
  }

  .search-bar {
    display: flex;
    align-items: center;
    padding: 4px 8px;
    gap: 6px;
    border-bottom: 1px solid var(--border-color);
    background: var(--bg-secondary);
    flex-shrink: 0;
  }

  .search-icon {
    flex-shrink: 0;
    color: var(--text-secondary);
  }

  .search-input {
    flex: 1;
    font-size: 11px;
    padding: 4px 6px;
    background: var(--bg-input);
    color: var(--text-primary);
    border: 1px solid transparent;
    border-radius: 4px;
    outline: none;
  }

  .search-input:focus {
    border-color: var(--accent);
  }

  .search-input::placeholder {
    color: var(--text-secondary);
  }

  .tree-toolbar {
    display: flex;
    align-items: center;
    padding: 3px 6px;
    gap: 1px;
    border-bottom: 1px solid var(--border-color);
    background: var(--bg-secondary);
    flex-shrink: 0;
  }

  .toolbar-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: none;
    color: var(--text-secondary);
    cursor: pointer;
    padding: 3px 5px;
    border-radius: 2px;
  }
  .toolbar-btn:hover {
    background: var(--bg-hover);
    color: var(--text-primary);
  }

  .toolbar-spacer {
    flex: 1;
  }

  .tree-body {
    overflow-y: auto;
    flex: 1;
    min-height: 0;
  }

  .tree-node {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 2px 8px;
    font-size: 12px;
    color: var(--text-primary);
    cursor: pointer;
    white-space: nowrap;
    min-height: 24px;
    transition: background 0.08s;
  }
  .tree-node:hover {
    background: var(--bg-hover);
  }
  .tree-node.selected {
    background: var(--bg-active);
    color: var(--text-bright);
  }
  .tree-node.drop-target {
    background: var(--accent-muted);
    outline: 1px dashed var(--accent);
  }
  .tree-node.drop-target-before {
    box-shadow: inset 0 2px 0 0 var(--accent);
  }
  .tree-node.drop-target-after {
    box-shadow: inset 0 -2px 0 0 var(--accent);
  }

  .drag-over-root {
    background: rgba(75, 139, 191, 0.05);
  }

  .folder-arrow {
    width: 14px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    color: var(--text-secondary);
  }
  .ping-dot {
    display: inline-block;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .conn-status {
    display: inline-flex;
    flex-shrink: 0;
    color: var(--text-secondary);
  }
  .conn-status.active { color: #4caf50; }
  .conn-status.connecting { color: #ffb300; }
  .conn-status.error { color: var(--danger, #f44747); }
  .conn-status .spinning { display: inline-flex; animation: spin 1s linear infinite; }
  @keyframes spin { from { transform: rotate(0deg); } to { transform: rotate(360deg); } }

  .folder-icon, .conn-icon {
    display: inline-flex;
    align-items: center;
    flex-shrink: 0;
    color: var(--text-secondary);
  }

  .node-name {
    flex: 1;
    min-width: 40px;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .folder-actions, .conn-actions {
    display: none;
    gap: 1px;
  }
  .tree-node:hover .folder-actions,
  .tree-node:hover .conn-actions {
    display: flex;
  }

  .micro-btn {
    background: none;
    border: none;
    color: var(--text-secondary);
    cursor: pointer;
    padding: 1px 3px;
    border-radius: 2px;
    display: inline-flex;
    align-items: center;
  }
  .micro-btn:hover {
    background: var(--bg-tertiary);
    color: var(--text-primary);
  }
  .micro-btn.danger:hover {
    color: var(--danger);
  }

  .inline-input {
    flex: 1;
    font-size: 12px;
    padding: 1px 4px;
    background: var(--bg-input);
    color: var(--text-primary);
    border: 1px solid var(--border-focus);
    border-radius: 2px;
    outline: none;
  }

  .tag-chips {
    display: inline-flex;
    gap: 3px;
    margin-left: 4px;
    flex-shrink: 0;
    max-width: 120px;
    overflow: hidden;
  }

  .tag-chip {
    font-size: 8px;
    padding: 0 3px;
    border-radius: 2px;
    color: #fff;
    line-height: 14px;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 50px;
  }

  .tag-more {
    font-size: 8px;
    color: var(--text-secondary);
    line-height: 14px;
  }
  .protocol-badge {
    font-size: 9px;
    padding: 0 4px;
    border-radius: 2px;
    background: var(--bg-tertiary, rgba(128, 128, 128, 0.2));
    color: var(--text-secondary);
    margin-left: 4px;
    white-space: nowrap;
  }

  .favorites-section {
    border-bottom: 1px solid var(--border-color);
    padding-bottom: 4px;
    margin-bottom: 4px;
  }

  .favorites-header {
    font-size: 10px;
    font-weight: 600;
    color: var(--text-secondary);
    text-transform: uppercase;
    letter-spacing: 0.5px;
    padding: 4px 8px;
  }

  .favorite-node {
    padding-left: 8px !important;
  }

  .empty-tree {
    padding: 16px;
    text-align: center;
    font-size: 12px;
    color: var(--text-secondary);
  }
</style>
