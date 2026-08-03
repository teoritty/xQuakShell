<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { listLocalPath, getUserHomeDir, removeLocalPath, mkdirLocalPath, createLocalFile, renameLocalPath, openFileWithSystem, type LocalNode } from '../api/localFs';
  import { startDownloadDrop, startLocalCopyDrop } from '../actions/transferActions';
  import { transferCompleted } from '../stores/appState';
  import { registerOsDropZone, resolveOsDropTarget, isFileDrag, isInternalFileDrag } from './osFileDrop';
  import { internalDragHighlight } from './dragHighlight';
  import { isInvalidMove } from './pathMove';
  import LocalFileTreeNode from './LocalFileTreeNode.svelte';
  import FileContextMenu from './FileContextMenu.svelte';
  import { openContextMenu, releaseContextMenu } from './contextMenuManager';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import OverflowToolbar from './OverflowToolbar.svelte';
  import { buildFilePanelToolbarItems, cycleSortState, type SortKey } from './filePanelToolbar';
  import { refreshesLocalPane } from './transferPresentation';
  import { ChevronUp, X } from 'lucide-svelte';

  const STORAGE_KEY = 'localfiletree-show-columns';
  const STORAGE_HIDDEN = 'localfiletree-show-hidden';


  let tree: Map<string, LocalNode[]> = new Map();
  let rawTree: Map<string, LocalNode[]> = new Map();
  let ctxMenu = { show: false, x: 0, y: 0, path: '', isDir: false, isEmptyArea: false };
  let expanded: Set<string> = new Set();
  let loading: Set<string> = new Set();
  let currentPath = '';
  let homeDir = '';
  let selectedPaths: Set<string> = new Set();
  let lastSelectedPath: string | null = null;
  let showPermissions = false;
  let showOwner = false;
  let showDate = false;
  let showHidden = false;
  let editingNewPath: string | null = null;
  let deleteConfirm = { show: false, path: '', name: '', isDir: false, childCount: 0, pathsToDelete: [] as string[] };
  let dragOverPath: string | null = null;
  let osDropOff: (() => void) | null = null;
  let rootEl: HTMLDivElement | null = null;
  type SortDir = 'asc' | 'desc';
  let sortEnabled = false;
  let sortKey: SortKey | null = null;
  let sortDir: SortDir = 'asc';

  function findNode(path: string): LocalNode | undefined {
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
        const children = await listLocalPath(path, true);
        childCount = children.length;
      } catch (_) {}
    }
    deleteConfirm = { show: true, path, name, isDir, childCount, pathsToDelete: [] };
  }

  async function confirmDelete() {
    const { path, pathsToDelete } = deleteConfirm;
    deleteConfirm = { ...deleteConfirm, show: false };
    const toDelete = pathsToDelete.length > 0 ? pathsToDelete : (path ? [path] : []);
    const affectedPaths = new Set<string>([currentPath]);
    for (const p of toDelete) {
      try {
        await removeLocalPath(p);
        const sep = p.includes('\\') ? '\\' : '/';
        const idx = p.lastIndexOf(sep);
        const parent = idx > 0 ? p.slice(0, idx) : homeDir;
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

  let pathInput = '';
  let pathInputEl: HTMLInputElement | null = null;
  let error = '';

  onMount(async () => {
    try {
      const stored = localStorage.getItem(STORAGE_KEY);
      if (stored) {
        const o = JSON.parse(stored);
        showPermissions = !!o.permissions;
        showOwner = !!o.owner;
        showDate = !!o.date;
      }
      showHidden = localStorage.getItem(STORAGE_HIDDEN) === '1';
    } catch (_) {}
    homeDir = (await getUserHomeDir()) || '';
    currentPath = homeDir;
    await loadDir(currentPath);
    expanded.add(currentPath);
    if (rootEl) osDropOff = registerOsDropZone({ el: rootEl, onDrop: handleOsFileDrop });
  });

  onDestroy(() => {
    if (osDropOff) osDropOff();
  });

  async function handleOsFileDrop(paths: string[], x: number, y: number) {
    if (!rootEl) return;
    const targetDir = resolveOsDropTarget(rootEl, x, y, currentPath);
    if (targetDir === null) return;
    await startLocalCopyDrop(paths, targetDir);
    await refreshPreservingState([targetDir, currentPath]);
  }

  async function loadDir(path: string) {
    if (loading.has(path)) return;
    loading.add(path);
    loading = loading;
    error = '';
    try {
      const nodes = (await listLocalPath(path, showHidden)) || [];
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
    const node = findNode(path);
    if (!node?.isDir) return;
    currentPath = path;
    expanded.add(path);
    expanded = expanded;
    selectedPaths = new Set([path]);
    lastSelectedPath = path;
    if (!tree.has(path)) await loadDir(path);
    tree = tree;
  }

  async function handlePathSubmit() {
    const trimmed = pathInput.trim();
    if (!trimmed) return;
    const nextPath = normalizePathInput(trimmed);
    const prevPath = currentPath;
    // listLocalPath normally swallows errors (returns []) and shows a global
    // banner; opt into rethrow so a non-existent path is caught here and the
    // view is reverted instead of navigating into an empty folder.
    try {
      const nodes = await listLocalPath(nextPath, showHidden, { rethrow: true, silence: () => true });
      rawTree.set(nextPath, nodes);
      tree.set(nextPath, applySort(nodes));
      currentPath = nextPath;
      if (!expanded.has(currentPath)) {
        expanded.add(currentPath);
        expanded = expanded;
      }
      tree = tree;
      error = '';
    } catch (e: any) {
      error = e?.message || String(e);
      currentPath = prevPath;
      pathInput = prevPath;
      return;
    }
  }

  $: if (!pathInputEl || document.activeElement !== pathInputEl) {
    pathInput = currentPath;
  }

  function isAtFilesystemRoot(path: string): boolean {
    const trimmed = path.replace(/[\\/]+$/, '');
    if (!trimmed || trimmed === '/') return true;
    if (/^[a-zA-Z]:\\?$/i.test(trimmed)) return true;
    return false;
  }

  function parentDirectory(path: string): string {
    if (isAtFilesystemRoot(path)) return path;
    const trimmed = path.replace(/[\\/]+$/, '');
    const idx = Math.max(trimmed.lastIndexOf('\\'), trimmed.lastIndexOf('/'));
    if (/^[a-zA-Z]:/.test(trimmed)) {
      if (idx <= 2) return `${trimmed.slice(0, 2)}\\`;
      return trimmed.slice(0, idx);
    }
    if (idx <= 0) return '/';
    return trimmed.slice(0, idx);
  }

  async function goUp() {
    if (isAtFilesystemRoot(currentPath)) return;
    const parent = parentDirectory(currentPath);
    if (!parent || parent === currentPath) return;
    currentPath = parent;
    await loadDir(currentPath);
    if (!expanded.has(currentPath)) {
      expanded.add(currentPath);
      expanded = expanded;
    }
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

  // Downloads land here, and so do local copies (an Explorer drop). Both are
  // recognised by their own honest kind. The directory to reload comes from
  // refreshDir, which every emitter fills with a real host path — single
  // downloads included, so nothing is derived from localPath any more.
  $: if ($transferCompleted && refreshesLocalPane($transferCompleted.kind)) {
    const t = $transferCompleted;
    transferCompleted.set(null);
    refreshPreservingState([t.refreshDir, currentPath]);
  }

  function formatSize(size: number): string {
    if (size < 1024) return `${size} B`;
    if (size < 1048576) return `${(size / 1024).toFixed(1)} KB`;
    if (size < 1073741824) return `${(size / 1048576).toFixed(1)} MB`;
    return `${(size / 1073741824).toFixed(1)} GB`;
  }

  function normalizePathInput(input: string): string {
    const looksWindows = input.includes('\\') || /^[a-zA-Z]:/.test(input);
    if (looksWindows) {
      let normalized = input.replace(/\//g, '\\').replace(/\\{2,}/g, '\\');
      if (/^[a-zA-Z]:$/.test(normalized)) return `${normalized}\\`;
      if (/^[a-zA-Z]:\\$/.test(normalized)) return normalized;
      normalized = normalized.replace(/\\+$/, '');
      return normalized || homeDir || '';
    }
    const normalized = input.replace(/\/{2,}/g, '/').replace(/\/+$/, '');
    return normalized || '/';
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

  function sortValue(node: LocalNode, key: SortKey): number | string {
    if (key === 'name') return node.name.toLowerCase();
    if (key === 'size') return node.size ?? 0;
    if (key === 'modTime') return parseTimestamp(node.modTime);
    return (node.owner || '').toLowerCase();
  }

  function applySort(nodes: LocalNode[]): LocalNode[] {
    if (!nodes) return [];
    const dir = sortDir === 'asc' ? 1 : -1;
    return [...nodes].sort((a, b) => {
      if (a.isDir !== b.isDir) return a.isDir ? -1 : 1;
      if (sortEnabled && sortKey) {
        const cmp = compareValues(sortValue(a, sortKey), sortValue(b, sortKey));
        if (cmp !== 0) return cmp * dir;
      }
      return a.name.localeCompare(b.name);
    });
  }

  function reapplySortToTree() {
    if (!sortEnabled || !sortKey) {
      tree = new Map(rawTree);
      return;
    }
    const next = new Map<string, LocalNode[]>();
    for (const [path, nodes] of rawTree.entries()) {
      next.set(path, applySort(nodes));
    }
    tree = next;
  }

  function toggleSort(nextKey: SortKey) {
    ({ sortEnabled, sortKey, sortDir } = cycleSortState(
      { sortEnabled, sortKey, sortDir },
      nextKey
    ));
    reapplySortToTree();
  }

  function handleDragOverPath(e: DragEvent, path: string) {
    // External OS file drags are handled by the window-level osFileDrop router
    // (via Wails). Do not stopPropagation here or the drop never reaches it.
    if (isFileDrag(e)) return;
    // Only claim genuine file-pane drags; other internal drags (e.g. a tile tab
    // dragged over this panel) must bubble to the tile layout for split/move.
    if (!isInternalFileDrag(e)) return;
    e.preventDefault();
    e.stopPropagation();
    if (e.dataTransfer) e.dataTransfer.dropEffect = 'copy';
    dragOverPath = path;
  }

  function handleDragLeave() {
    dragOverPath = null;
  }

  async function handleDrop(e: DragEvent, targetDir: string) {
    // External OS file drops bubble up to the osFileDrop router; leave them alone.
    if (isFileDrag(e)) return;
    // Ignore non-file internal drags (e.g. tile-tab drags) so they reach the tile.
    if (!isInternalFileDrag(e)) return;
    e.preventDefault();
    e.stopPropagation();
    dragOverPath = null;
    if (!e.dataTransfer) return;
    const dropSessionId = e.dataTransfer.getData('text/session-id');
    const localPathsJson = e.dataTransfer.getData('text/local-selected-paths');
    const localPaths = localPathsJson
      ? ((): string[] => { try { return JSON.parse(localPathsJson); } catch { return []; } })()
      : null;
    const localPath = localPaths ? null : e.dataTransfer.getData('text/local-path') || null;
    const remotePathsJson = e.dataTransfer.getData('text/selected-paths');
    const remotePaths = remotePathsJson
      ? ((): string[] => { try { return JSON.parse(remotePathsJson); } catch { return []; } })()
      : null;
    const remotePath = remotePaths ? null : e.dataTransfer.getData('text/remote-path') || null;

    const locals = localPaths && localPaths.length > 0 ? localPaths : (localPath ? [localPath] : []);
    if (locals.length > 0) {
      const sep = targetDir.includes('\\') ? '\\' : '/';
      const srcParents: string[] = [];
      for (const lp of locals) {
        const base = lp.split(/[\\/]/).pop() || 'item';
        const destPath = targetDir.endsWith(sep) ? targetDir + base : targetDir + sep + base;
        if (!isInvalidMove(lp, targetDir)) {
          try {
            await renameLocalPath(lp, destPath);
            const srcSep = lp.includes('\\') ? '\\' : '/';
            const srcParent = lp.split(srcSep).slice(0, -1).join(srcSep) || srcSep;
            if (!srcParents.includes(srcParent)) srcParents.push(srcParent);
          } catch (err: any) {
            error = err?.message || String(err);
          }
        }
      }
      if (srcParents.length > 0) await refreshPreservingState([targetDir, currentPath, ...srcParents]);
      return;
    }
    const remotes = remotePaths && remotePaths.length > 0 ? remotePaths : (remotePath ? [remotePath] : []);
    if (remotes.length > 0 && dropSessionId) {
      await startDownloadDrop(dropSessionId, remotes, targetDir);
      await refreshPreservingState([targetDir, currentPath]);
    }
  }

  function handleDragStartFile(e: DragEvent, node: LocalNode) {
    if (!e.dataTransfer) return;
    e.dataTransfer.effectAllowed = 'copy';
    const multi = selectedPaths.has(node.path) && selectedPaths.size > 1;
    if (multi) {
      e.dataTransfer.setData('text/local-selected-paths', JSON.stringify([...selectedPaths]));
    } else {
      e.dataTransfer.setData('text/local-path', node.path);
    }
    e.dataTransfer.setData('text/is-dir', node.isDir ? '1' : '0');
  }

  function showContextMenu(e: MouseEvent, path: string, isDir: boolean, isEmptyArea: boolean) {
    e.preventDefault();
    e.stopPropagation();
    ctxMenu = { show: true, x: e.clientX, y: e.clientY, path, isDir, isEmptyArea };
    openContextMenu(closeContextMenu);
  }

  function closeContextMenu() {
    releaseContextMenu(closeContextMenu);
    ctxMenu = { ...ctxMenu, show: false };
  }

  async function handleCtxEdit() {
    if (!ctxMenu.path || ctxMenu.isDir) return;
    closeContextMenu();
    try {
      await openFileWithSystem(ctxMenu.path);
    } catch (e: any) {
      error = e?.message || String(e);
    }
  }

  async function handleCtxDelete() {
    if (!ctxMenu.path) return;
    const name = ctxMenu.path.split(/[\\/]/).pop() || ctxMenu.path;
    closeContextMenu();
    await requestDelete(ctxMenu.path, ctxMenu.isDir, name);
  }

  function uniqueName(parentPath: string, base: string): string {
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
    const sep = parentPath.includes('\\') ? '\\' : '/';
    const baseName = uniqueName(parentPath, 'New Folder');
    const dirPath = (parentPath.endsWith(sep) ? parentPath : parentPath + sep) + baseName;
    try {
      await mkdirLocalPath(dirPath);
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
    editingNewPath = dirPath;
    tree = tree;
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
    const sep = parentPath.includes('\\') ? '\\' : '/';
    const baseName = uniqueName(parentPath, 'New File');
    const filePath = (parentPath.endsWith(sep) ? parentPath : parentPath + sep) + baseName;
    try {
      await createLocalFile(filePath);
    } catch (e: any) {
      error = e?.message || String(e);
      closeContextMenu();
      return;
    }
    closeContextMenu();
    await loadDir(parentPath);
    editingNewPath = filePath;
    tree = tree;
  }

  function handleCtxRename() {
    if (ctxMenu.isEmptyArea || !ctxMenu.path) {
      closeContextMenu();
      return;
    }
    editingNewPath = ctxMenu.path;
    closeContextMenu();
  }

  async function handleRenameConfirm(oldPath: string, newName: string) {
    if (!newName.trim()) {
      editingNewPath = null;
      return;
    }
    const sep = oldPath.includes('\\') ? '\\' : '/';
    const lastSep = Math.max(oldPath.lastIndexOf(sep), oldPath.lastIndexOf('/'));
    const parent = lastSep > 0 ? oldPath.substring(0, lastSep) : homeDir;
    const newFullPath = (parent.endsWith(sep) ? parent : parent + sep) + newName.trim();
    if (newFullPath === oldPath) {
      editingNewPath = null;
      return;
    }
    try {
      await renameLocalPath(oldPath, newFullPath);
    } catch (e: any) {
      error = e?.message || String(e);
      editingNewPath = null;
      return;
    }
    editingNewPath = null;
    await loadDir(parent || currentPath);
    tree = tree;
  }

  function handleRenameCancel() {
    editingNewPath = null;
  }

  $: toolbarItems = buildFilePanelToolbarItems({
    showPermissions,
    showOwner,
    showDate,
    showHidden,
    sortEnabled,
    sortKey,
    sortDir,
    togglePermissions,
    toggleOwner,
    toggleDate,
    toggleHidden,
    toggleSort,
    refresh,
  });
</script>

<svelte:window on:click={closeContextMenu} />
<div
  class="file-tree"
  class:internal-drop-active={internalDragHighlight(dragOverPath, currentPath) === 'pane'}
  bind:this={rootEl}
>
  <div class="panel-header">
    <span>Local Files</span>
    <OverflowToolbar items={toolbarItems} />
  </div>
  <div class="path-bar">
    <input
      bind:this={pathInputEl}
      bind:value={pathInput}
      on:keydown={(e) => e.key === 'Enter' && handlePathSubmit()}
      on:blur={() => pathInput = currentPath}
      placeholder="C:\"
    />
  </div>

  {#if error}
    <div class="tree-error">
      <span class="tree-error-msg">{error}</span>
      <button class="tree-error-close" title="Dismiss" on:click={() => (error = '')}><X size={12} /></button>
    </div>
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
    {#if currentPath && !isAtFilesystemRoot(currentPath)}
      <div class="parent-node" on:click={goUp} on:keydown={(e) => e.key === 'Enter' && goUp()} role="button" tabindex="0">
        <span class="node-icon"><ChevronUp size={12} /></span>
        <span class="node-name">..</span>
      </div>
    {/if}
    {#each tree.get(currentPath) || [] as node (node.path)}
      <LocalFileTreeNode
        {node}
        {tree}
        {expanded}
        {loading}
        selectedPaths={selectedPaths}
        onToggle={toggleDir}
        onSelect={selectNode}
        onNavigate={navigateInto}
        onDrop={handleDrop}
        onDragOverPath={(e, p) => handleDragOverPath(e, p)}
        dropTargetPath={dragOverPath}
        onDragStartFile={handleDragStartFile}
        onContextMenu={(e, n) => showContextMenu(e, n.path, n.isDir, false)}
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
    on:rename={handleCtxRename}
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

  .tree-error {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 10px;
    font-size: 11px;
    color: var(--danger);
    background: rgba(211, 47, 47, 0.1);
    border-bottom: 1px solid var(--border-color);
  }

  .tree-error-msg {
    flex: 1;
    min-width: 0;
    word-break: break-word;
  }

  .tree-error-close {
    flex-shrink: 0;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 2px;
    color: var(--danger);
    background: transparent;
    border: none;
    border-radius: 3px;
    cursor: pointer;
  }

  .tree-error-close:hover {
    background: rgba(211, 47, 47, 0.18);
  }

  .tree-body {
    overflow-y: auto;
    flex: 1;
    padding: 4px 0;
  }

  /* Drag-over highlight applied by osFileDrop's router while an OS file is
     dragged over this pane (see registerOsDropZone). */
  .file-tree:global(.os-drop-active) {
    outline: 2px dashed var(--accent);
    outline-offset: -3px;
    background: rgba(100, 150, 255, 0.08);
  }

  /* Same fill for an internal pane-to-pane drag whose drop would land in this
     pane's current directory (cursor over a file row or empty space). Driven by
     our own drag state rather than the OS router, which only sees drags carrying
     real OS files — a distinction WebView2 blurred and WebKitGTK does not. */
  .file-tree.internal-drop-active {
    outline: 2px dashed var(--accent);
    outline-offset: -3px;
    background: rgba(100, 150, 255, 0.08);
  }

  /* Folder row highlighted when an OS file is dragged directly over it (drop
     targets that folder rather than the current directory). */
  :global(.node-row.os-drop-active) {
    background: rgba(100, 150, 255, 0.22);
    outline: 1px solid var(--accent);
    outline-offset: -1px;
  }

</style>
