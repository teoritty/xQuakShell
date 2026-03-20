<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import type { RemoteNode } from '../stores/appState';
  import { listPath, removePath, mkdirPath, createFilePath, renamePath, downloadFile, getTempDir, openFileWithSystem, startFileWatch, getSettings } from '../stores/api';
  import { editingFiles, transferCompleted } from '../stores/appState';
  import FileTreeNode from './FileTreeNode.svelte';
  import FileContextMenu from './FileContextMenu.svelte';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import { RefreshCw, Loader2, ChevronUp, Shield, User, Calendar, Eye, ArrowUpDown } from 'lucide-svelte';

  const STORAGE_KEY = 'filetree-show-columns';
  const STORAGE_HIDDEN = 'filetree-show-hidden';

  export let sessionId: string;
  export let onDropUpload: ((localPath: string, remotePath: string) => void) | undefined = undefined;

  let tree: Map<string, RemoteNode[]> = new Map();
  let rawTree: Map<string, RemoteNode[]> = new Map();
  let ctxMenu = { show: false, x: 0, y: 0, path: '', isDir: false, isEmptyArea: false, size: 0 };
  let expanded: Set<string> = new Set();
  let loading: Set<string> = new Set();
  let currentPath = '/';
  let selectedPaths: Set<string> = new Set();
  let lastSelectedPath: string | null = null;
  let showPermissions = false;
  let showOwner = false;
  let showDate = false;
  let showHidden = false;
  let editingNewPath: string | null = null;
  let deleteConfirm = { show: false, path: '', name: '', isDir: false, childCount: 0, pathsToDelete: [] as string[] };
  let error = '';
  let ready = false;
  let eventOff: (() => void) | null = null;
  let dragOverPath: string | null = null;
  type SortKey = 'name' | 'size' | 'modTime' | 'owner';
  type SortDir = 'asc' | 'desc';
  let sortEnabled = false;
  let sortKey: SortKey | null = null;
  let sortDir: SortDir = 'desc';

  onMount(() => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored) {
        const o = JSON.parse(stored);
        showPermissions = !!o.permissions;
        showOwner = !!o.owner;
        showDate = !!o.date;
      }
    } catch (_) {}
    const rt = (window as any).runtime;
    if (rt) {
      const handler = (data: { sessionId: string; initialPath?: string }) => {
        if (data.sessionId === sessionId) {
          ready = true;
          if (data.initialPath) {
            currentPath = data.initialPath;
          }
          refresh();
        }
      };
      rt.EventsOn('SFTPReady', handler);
      eventOff = () => rt.EventsOff('SFTPReady');
    }
  });

  onDestroy(() => {
    if (eventOff) eventOff();
  });

  async function loadDir(path: string) {
    if (loading.has(path)) return;
    loading.add(path);
    loading = loading;
    error = '';
    try {
      const nodes = await listPath(sessionId, path);
      rawTree.set(path, nodes);
      tree.set(path, applySort(nodes));
      tree = tree;
    } catch (e: any) {
      error = e?.message || String(e);
    } finally {
      loading.delete(path);
      loading = loading;
    }
  }

  async function goUp() {
    const parent = currentPath.replace(/\/[^/]+$/, '') || '/';
    if (parent !== currentPath) {
      currentPath = parent;
      await loadDir(currentPath);
      if (!expanded.has(currentPath)) {
        expanded.add(currentPath);
        expanded = expanded;
      }
      tree = tree;
    }
  }

  async function toggleDir(path: string) {
    if (expanded.has(path)) {
      expanded.delete(path);
      expanded = expanded;
    } else {
      expanded.add(path);
      expanded = expanded;
      if (!tree.has(path)) {
        await loadDir(path);
      }
    }
  }

  function selectNode(path: string, e?: MouseEvent) {
    const nodes = tree.get(currentPath) || [];
    if (e?.ctrlKey || e?.metaKey) {
      const next = new Set(selectedPaths);
      if (next.has(path)) next.delete(path);
      else next.add(path);
      selectedPaths = next;
      lastSelectedPath = path;
    } else if (e?.shiftKey) {
      const idx = nodes.findIndex((n) => n.path === path);
      const lastIdx = lastSelectedPath != null ? nodes.findIndex((n) => n.path === lastSelectedPath) : -1;
      const next = new Set(selectedPaths);
      const [lo, hi] = lastIdx >= 0 ? (idx < lastIdx ? [idx, lastIdx] : [lastIdx, idx]) : [idx, idx];
      for (let i = lo; i <= hi; i++) next.add(nodes[i].path);
      selectedPaths = next;
    } else {
      selectedPaths = new Set([path]);
      lastSelectedPath = path;
    }
  }

  async function navigateInto(path: string) {
    const node = (tree.get(currentPath) || []).find((n) => n.path === path);
    if (!node?.isDir) return;
    currentPath = path;
    expanded.add(path);
    expanded = expanded;
    selectedPaths = new Set([path]);
    lastSelectedPath = path;
    if (!tree.has(path)) await loadDir(path);
    tree = tree;
  }

  export async function refresh() {
    tree = new Map();
    rawTree = new Map();
    expanded = new Set();
    await loadDir(currentPath);
    expanded.add(currentPath);
  }

  export async function refreshPreservingState(affectedPaths?: string[]) {
    const paths = affectedPaths && affectedPaths.length > 0
      ? [...new Set(affectedPaths)]
      : [currentPath];
    for (const p of paths) {
      await loadDir(p);
    }
    tree = tree;
  }

  $: if (ready && $transferCompleted?.direction === 'upload' && $transferCompleted?.sessionId === sessionId) {
    const t = $transferCompleted;
    transferCompleted.set(null);
    const remoteParent = t.remotePath.replace(/\/[^/]+$/, '') || '/';
    refreshPreservingState([remoteParent, currentPath]);
  }

  function formatSize(size: number): string {
    if (size < 1024) return `${size} B`;
    if (size < 1048576) return `${(size / 1024).toFixed(1)} KB`;
    if (size < 1073741824) return `${(size / 1048576).toFixed(1)} MB`;
    return `${(size / 1073741824).toFixed(1)} GB`;
  }

  function parseTimestamp(value?: string): number {
    if (!value) return -1;
    const ts = Date.parse(value);
    return Number.isFinite(ts) ? ts : -1;
  }

  function compareValues(a: number | string, b: number | string): number {
    if (typeof a === 'string' && typeof b === 'string') return a.localeCompare(b);
    return Number(a) - Number(b);
  }

  function sortValue(node: RemoteNode, key: SortKey): number | string {
    if (key === 'name') return node.name.toLowerCase();
    if (key === 'size') return node.size ?? 0;
    if (key === 'modTime') return parseTimestamp(node.modTime);
    return (node.owner || node.group || '').toLowerCase();
  }

  function applySort(nodes: RemoteNode[]): RemoteNode[] {
    if (!sortEnabled || !sortKey) return nodes;
    const dir = sortDir === 'asc' ? 1 : -1;
    return [...nodes].sort((a, b) => {
      const cmp = compareValues(sortValue(a, sortKey), sortValue(b, sortKey));
      if (cmp !== 0) return cmp * dir;
      return a.name.localeCompare(b.name) * dir;
    });
  }

  function reapplySortToTree() {
    if (!sortEnabled || !sortKey) {
      tree = new Map(rawTree);
      return;
    }
    const next = new Map<string, RemoteNode[]>();
    for (const [path, nodes] of rawTree.entries()) {
      next.set(path, applySort(nodes));
    }
    tree = next;
  }

  function toggleSort(nextKey: SortKey) {
    if (!sortEnabled || sortKey !== nextKey) {
      sortEnabled = true;
      sortKey = nextKey;
      sortDir = 'desc';
      reapplySortToTree();
      return;
    }
    if (sortDir === 'desc') {
      sortDir = 'asc';
      reapplySortToTree();
      return;
    }
    sortEnabled = false;
    sortKey = null;
    sortDir = 'desc';
    reapplySortToTree();
  }

  function sortIndicator(key: SortKey): string {
    if (!sortEnabled || sortKey !== key) return '';
    return sortDir === 'desc' ? ' ↓' : ' ↑';
  }

  function handleDragOverPath(e: DragEvent, path: string) {
    e.preventDefault();
    e.stopPropagation();
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy';
    dragOverPath = path;
  }

  function handleDragLeave() {
    dragOverPath = null;
  }

  async function handleDrop(e: DragEvent, targetDir: string) {
    e.preventDefault();
    e.stopPropagation();
    dragOverPath = null;
    if (!e.dataTransfer) return;
    const dropSessionId = e.dataTransfer.getData('text/session-id');
    const selectedPathsJson = e.dataTransfer.getData('text/selected-paths');
    const remotePaths = selectedPathsJson
      ? ((): string[] => { try { return JSON.parse(selectedPathsJson); } catch { return []; } })()
      : null;
    const remotePath = remotePaths ? null : e.dataTransfer.getData('text/remote-path') || null;
    const localPathsJson = e.dataTransfer.getData('text/local-selected-paths');
    const localPaths = localPathsJson
      ? ((): string[] => { try { return JSON.parse(localPathsJson); } catch { return []; } })()
      : null;
    const localPath = localPaths ? null : e.dataTransfer.getData('text/local-path') || null;

    const remotes = remotePaths && remotePaths.length > 0 ? remotePaths : (remotePath ? [remotePath] : []);
    if (remotes.length > 0 && dropSessionId === sessionId) {
      const srcParents: string[] = [];
      for (const rp of remotes) {
        const base = rp.split('/').filter(Boolean).pop() || 'item';
        const destPath = targetDir === '/' ? `/${base}` : `${targetDir}/${base}`;
        if (rp !== destPath) {
          try {
            await renamePath(sessionId, rp, destPath);
            const srcParent = rp.replace(/\/[^/]+$/, '') || '/';
            if (!srcParents.includes(srcParent)) srcParents.push(srcParent);
          } catch (err: any) {
            error = err?.message || String(err);
          }
        }
      }
      if (srcParents.length > 0) await refreshPreservingState([targetDir, currentPath, ...srcParents]);
      return;
    }
    const locals = localPaths && localPaths.length > 0 ? localPaths : (localPath ? [localPath] : []);
    if (locals.length > 0 && onDropUpload) {
      for (const lp of locals) {
        const fileName = lp.split(/[\\/]/).pop() || 'file';
        const remoteDest = targetDir === '/' ? `/${fileName}` : `${targetDir}/${fileName}`;
        onDropUpload(lp, remoteDest);
      }
    }
  }

  function handleDragStartFile(e: DragEvent, node: RemoteNode) {
    if (!e.dataTransfer) return;
    e.dataTransfer.effectAllowed = 'copy';
    const multi = selectedPaths.has(node.path) && selectedPaths.size > 1;
    if (multi) {
      e.dataTransfer.setData('text/selected-paths', JSON.stringify([...selectedPaths]));
    } else {
      e.dataTransfer.setData('text/remote-path', node.path);
    }
    e.dataTransfer.setData('text/session-id', sessionId);
    e.dataTransfer.setData('text/is-dir', node.isDir ? '1' : '0');
  }

  function showContextMenu(e: MouseEvent, path: string, isDir: boolean, isEmptyArea: boolean, size = 0) {
    e.preventDefault();
    e.stopPropagation();
    ctxMenu = { show: true, x: e.clientX, y: e.clientY, path, isDir, isEmptyArea, size };
  }

  function closeContextMenu() {
    ctxMenu = { ...ctxMenu, show: false };
  }

  function findNode(path: string): RemoteNode | undefined {
    for (const [, nodes] of tree) {
      const n = nodes.find((x) => x.path === path);
      if (n) return n;
    }
    return undefined;
  }

  async function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Delete' && selectedPaths.size > 0) {
      e.preventDefault();
      const paths = Array.from(selectedPaths);
      if (paths.length === 1) {
        const node = findNode(paths[0]);
        if (node) await requestDelete(paths[0], node.isDir, node.name);
      } else {
        deleteConfirm = {
          show: true,
          path: '',
          name: '',
          isDir: false,
          childCount: paths.length,
          pathsToDelete: paths,
        };
      }
    }
  }

  async function requestDelete(path: string, isDir: boolean, name: string) {
    let childCount = 0;
    if (isDir) {
      try {
        const children = await listPath(sessionId, path);
        childCount = children.length;
      } catch (_) {}
    }
    deleteConfirm = { show: true, path, name, isDir, childCount, pathsToDelete: [] };
  }

  async function handleCtxDelete() {
    if (!ctxMenu.path) return;
    const name = ctxMenu.path.split('/').pop() || ctxMenu.path;
    closeContextMenu();
    await requestDelete(ctxMenu.path, ctxMenu.isDir, name);
  }

  async function confirmDelete() {
    const { path, pathsToDelete } = deleteConfirm;
    deleteConfirm = { ...deleteConfirm, show: false };
    const toDelete = (pathsToDelete && pathsToDelete.length > 0) ? pathsToDelete : (path ? [path] : []);
    const affectedPaths = new Set<string>([currentPath]);
    for (const p of toDelete) {
      try {
        await removePath(sessionId, p);
        const parent = p.replace(/\/[^/]+$/, '') || '/';
        if (parent) affectedPaths.add(parent);
      } catch (e: any) {
        error = e?.message || String(e);
      }
    }
    selectedPaths = new Set();
    lastSelectedPath = null;
    await refreshPreservingState([...affectedPaths]);
  }

  function cancelDelete() {
    deleteConfirm = { ...deleteConfirm, show: false };
  }

  const MAX_EDIT_SIZE = 5 * 1024 * 1024; // 5 MB

  async function handleCtxEdit() {
    if (!ctxMenu.path || ctxMenu.isDir) return;
    const remotePath = ctxMenu.path;
    const size = ctxMenu.size;
    closeContextMenu();
    if (size > MAX_EDIT_SIZE) {
      error = `File too large to edit (max ${MAX_EDIT_SIZE / 1024 / 1024} MB)`;
      return;
    }
    try {
      const tempDir = await getTempDir();
      if (!tempDir) throw new Error('Could not get temp directory');
      await downloadFile(sessionId, remotePath, tempDir);
      const sep = tempDir.includes('\\') ? '\\' : '/';
      const fileName = remotePath.split('/').pop() || 'file';
      const localPath = tempDir.endsWith(sep) ? tempDir + fileName : tempDir + sep + fileName;
      const settings = await getSettings();
      const editorPath = settings?.externalEditorPath?.trim() || '';
      editingFiles.update((m) => {
        const next = new Map(m);
        next.set(localPath, { sessionId, remotePath });
        return next;
      });
      await openFileWithSystem(localPath, editorPath);
      startFileWatch(localPath);
    } catch (e: any) {
      error = e?.message || String(e);
    }
  }

  function uniqueName(parentPath: string, base: string, isDir: boolean): string {
    const existing = (tree.get(parentPath) || []).map((n) => n.name);
    let name = base;
    let i = 1;
    while (existing.includes(name)) {
      name = `${base} (${++i})`;
    }
    return name;
  }

  async function handleCtxNewFolder() {
    const parentPath = ctxMenu.isEmptyArea ? currentPath : ctxMenu.path;
    const baseName = uniqueName(parentPath, 'New Folder', true);
    try {
      await mkdirPath(sessionId, parentPath, baseName);
    } catch (e: any) {
      error = e?.message || String(e);
      closeContextMenu();
      return;
    }
    closeContextMenu();
    if (ctxMenu.isDir) {
      expanded.add(ctxMenu.path);
      expanded = expanded;
      await loadDir(ctxMenu.path);
    } else {
      await loadDir(currentPath);
    }
    const newPath = parentPath === '/' ? `/${baseName}` : `${parentPath}/${baseName}`;
    editingNewPath = newPath;
    tree = tree;
  }

  async function handlePathSubmit() {
    const trimmed = pathInput.trim();
    if (!trimmed) return;
    const normalized = trimmed.replace(/\\/g, '/').replace(/\/+/g, '/').replace(/\/$/, '') || '/';
    currentPath = normalized.startsWith('/') ? normalized : `/${normalized}`;
    await loadDir(currentPath);
    if (!expanded.has(currentPath)) {
      expanded.add(currentPath);
      expanded = expanded;
    }
    tree = tree;
  }

  let pathInput = '';
  let pathInputEl: HTMLInputElement | null = null;

  $: if (ready && (!pathInputEl || document.activeElement !== pathInputEl)) {
    pathInput = currentPath;
  }

  function saveColumnPrefs() {
    try {
      localStorage.setItem(STORAGE_KEY, JSON.stringify({ permissions: showPermissions, owner: showOwner, date: showDate }));
    } catch (_) {}
  }

  function togglePermissions() { showPermissions = !showPermissions; saveColumnPrefs(); }
  function toggleOwner() { showOwner = !showOwner; saveColumnPrefs(); }
  function toggleDate() { showDate = !showDate; saveColumnPrefs(); }
  function toggleHidden() {
    showHidden = !showHidden;
    try { localStorage.setItem(STORAGE_HIDDEN, showHidden ? '1' : '0'); } catch (_) {}
    refreshPreservingState([...expanded, currentPath]);
  }

  async function handleCtxNewFile() {
    if (!ctxMenu.isDir) return;
    const parentPath = ctxMenu.path;
    const baseName = uniqueName(parentPath, 'New File', false);
    try {
      await createFilePath(sessionId, parentPath, baseName);
    } catch (e: any) {
      error = e?.message || String(e);
      closeContextMenu();
      return;
    }
    closeContextMenu();
    await loadDir(parentPath);
    const newPath = parentPath === '/' ? `/${baseName}` : `${parentPath}/${baseName}`;
    editingNewPath = newPath;
    tree = tree;
  }

  async function handleRenameConfirm(oldPath: string, newName: string) {
    if (!newName.trim()) {
      editingNewPath = null;
      return;
    }
    const parent = oldPath.replace(/\/[^/]+$/, '') || '/';
    const newPath = parent === '/' ? `/${newName.trim()}` : `${parent}/${newName.trim()}`;
    if (newPath === oldPath) {
      editingNewPath = null;
      return;
    }
    try {
      await renamePath(sessionId, oldPath, newPath);
    } catch (e: any) {
      error = e?.message || String(e);
      return;
    }
    editingNewPath = null;
    await loadDir(parent);
    tree = tree;
  }

  function handleRenameCancel() {
    editingNewPath = null;
  }
</script>

<svelte:window on:click={closeContextMenu} />
<div class="file-tree">
  <div class="panel-header">
    <span>Remote Files</span>
    <div class="actions">
      <button class="column-toggle" class:active={showPermissions} on:click={togglePermissions} title="Permissions"><Shield size={12} /></button>
      <button class="column-toggle" class:active={showOwner} on:click={toggleOwner} title="Owner"><User size={12} /></button>
      <button class="column-toggle" class:active={showDate} on:click={toggleDate} title="Date"><Calendar size={12} /></button>
      <button class="column-toggle" class:active={showHidden} on:click={toggleHidden} title="Show hidden"><Eye size={12} /></button>
      <button class="sort-toggle" class:active={sortEnabled && sortKey === 'name'} on:click={() => toggleSort('name')} title="Sort by name"><ArrowUpDown size={12} /> N{sortIndicator('name')}</button>
      <button class="sort-toggle" class:active={sortEnabled && sortKey === 'size'} on:click={() => toggleSort('size')} title="Sort by size"><ArrowUpDown size={12} /> S{sortIndicator('size')}</button>
      <button class="sort-toggle" class:active={sortEnabled && sortKey === 'modTime'} on:click={() => toggleSort('modTime')} title="Sort by date"><ArrowUpDown size={12} /> D{sortIndicator('modTime')}</button>
      <button class="sort-toggle" class:active={sortEnabled && sortKey === 'owner'} on:click={() => toggleSort('owner')} title="Sort by owner"><ArrowUpDown size={12} /> O{sortIndicator('owner')}</button>
      <button on:click={refresh} title="Refresh" disabled={!ready}><RefreshCw size={12} /></button>
    </div>
  </div>

  {#if ready}
    <div class="path-bar">
      <input
        bind:this={pathInputEl}
        bind:value={pathInput}
        on:keydown={(e) => e.key === 'Enter' && handlePathSubmit()}
        on:blur={() => pathInput = currentPath}
        placeholder="/"
      />
    </div>
  {/if}

  {#if !ready}
    <div class="tree-loading"><Loader2 size={14} /> Connecting SFTP...</div>
  {:else if error}
    <div class="tree-error">{error}</div>
  {/if}

  <div
    class="tree-body"
    on:dragover={(e) => handleDragOverPath(e, currentPath)}
    on:dragleave={handleDragLeave}
    on:drop={(e) => handleDrop(e, currentPath)}
    on:contextmenu={(e) => showContextMenu(e, currentPath, true, true)}
    on:keydown={handleKeydown}
    role="tree"
    tabindex="0"
  >
    {#if ready && currentPath !== '/'}
      <div class="parent-node" on:click={goUp} on:keydown={(e) => e.key === 'Enter' && goUp()} role="button" tabindex="0">
        <span class="node-icon"><ChevronUp size={12} /></span>
        <span class="node-name">..</span>
      </div>
    {/if}
    {#each (tree.get(currentPath) || []).filter((n) => showHidden || !n.name.startsWith('.')) as node (node.path)}
      <FileTreeNode
        {node}
        {tree}
        {expanded}
        {loading}
        {sessionId}
        selectedPaths={selectedPaths}
        onToggle={toggleDir}
        onSelect={selectNode}
        onNavigate={navigateInto}
        onDrop={handleDrop}
        onDragOverPath={(e, p) => handleDragOverPath(e, p)}
        dropTargetPath={dragOverPath}
        onDragStartFile={handleDragStartFile}
        onContextMenu={(e, n) => showContextMenu(e, n.path, n.isDir, false, n.size)}
        formatSize={formatSize}
        {showPermissions}
        {showOwner}
        {showDate}
        editingNewPath={editingNewPath}
        onRenameConfirm={handleRenameConfirm}
        onRenameCancel={handleRenameCancel}
      />
    {/each}
  </div>

  <FileContextMenu
    x={ctxMenu.x}
    y={ctxMenu.y}
    show={ctxMenu.show}
    isDir={ctxMenu.isDir}
    isEmptyArea={ctxMenu.isEmptyArea}
    on:delete={handleCtxDelete}
    on:newFolder={handleCtxNewFolder}
    on:newFile={handleCtxNewFile}
    on:edit={handleCtxEdit}
  />

  <ConfirmDialog
    show={deleteConfirm.show}
    title={deleteConfirm.pathsToDelete.length > 1 || deleteConfirm.childCount > 0 ? 'Delete items?' : 'Delete?'}
    message={deleteConfirm.pathsToDelete.length > 0
      ? `You are deleting ${deleteConfirm.pathsToDelete.length} item(s). This action cannot be undone.`
      : deleteConfirm.childCount > 0
        ? `You are deleting "${deleteConfirm.name}" and ${deleteConfirm.childCount} item(s) inside. This action cannot be undone.`
        : `Delete "${deleteConfirm.name}"?`}
    critical={deleteConfirm.pathsToDelete.length > 1 || deleteConfirm.childCount > 0}
    requireCheckbox={deleteConfirm.pathsToDelete.length > 1 || deleteConfirm.childCount > 0}
    checkboxLabel="I understand"
    confirmLabel="Delete"
    on:confirm={confirmDelete}
    on:cancel={cancelDelete}
  />
</div>

<style>
  .file-tree {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }

  .tree-loading {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 12px;
    font-size: 11px;
    color: var(--text-secondary);
  }

  .tree-error {
    padding: 8px 10px;
    font-size: 11px;
    color: var(--danger);
    background: rgba(211, 47, 47, 0.1);
    border-bottom: 1px solid var(--border-color);
  }

  .path-bar {
    padding: 2px 8px;
    border-bottom: 1px solid var(--border-color);
  }

  .path-bar input {
    width: 100%;
    padding: 4px 6px;
    font-size: 11px;
    color: var(--text-primary);
    background: var(--bg-secondary);
    border: 1px solid transparent;
    border-radius: 4px;
    outline: none;
  }

  .path-bar input:focus {
    border-color: var(--accent);
  }

  .parent-node {
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 2px 8px;
    cursor: pointer;
    font-size: 12px;
    user-select: none;
    transition: background 0.1s;
  }

  .parent-node:hover {
    background: var(--bg-hover);
  }

  .parent-node .node-icon {
    display: inline-flex;
    flex-shrink: 0;
    color: var(--text-secondary);
  }

  .parent-node .node-name {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .tree-body {
    overflow-y: auto;
    flex: 1;
    padding: 4px 0;
  }

  .column-toggle {
    opacity: 0.6;
  }
  .column-toggle.active {
    opacity: 1;
    color: var(--accent);
  }
  .sort-toggle {
    display: inline-flex;
    align-items: center;
    gap: 2px;
    font-size: 10px;
    opacity: 0.85;
  }
  .sort-toggle.active {
    color: var(--accent);
    opacity: 1;
  }
</style>
