<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import type { RemoteNode } from '../stores/appState';
  import { listPath, removePath, mkdirPath, createFilePath, renamePath, downloadFile } from '../api/remoteFs';
  import { startUploadDrop } from '../actions/transferActions';
  import { getTempDir, openFileWithSystem, startFileWatch } from '../api/localFs';
  import { getSettings } from '../actions/settingsActions';
  import { sftpReadyPaths } from '../events/subscribe';
  import { editingFiles, transferCompleted } from '../stores/appState';
  import { registerOsDropZone, resolveOsDropTarget, isFileDrag, isInternalFileDrag } from './osFileDrop';
  import { internalDragHighlight } from './dragHighlight';
  import { isInvalidMove } from './pathMove';
  import { keepsPaneSelection } from './paneSelection';
  import FileTreeNode from './FileTreeNode.svelte';
  import FileContextMenu from './FileContextMenu.svelte';
  import { openContextMenu, releaseContextMenu } from './contextMenuManager';
  import ConfirmDialog from './ConfirmDialog.svelte';
  import PermissionsDialog from './PermissionsDialog.svelte';
  import { buildFilePanelToolbarItems, cycleSortState, type SortKey } from './filePanelToolbar';
  import { refreshesRemotePane, remoteRefreshDirs } from './transferPresentation';
  import type { SortState } from './fileTree/types';
  import { formatSize, applySort, sortTree } from './fileTree/sorting';
  import { selectNode as nextSelection, clearSelection, findNode as findNodeIn } from './fileTree/selection';
  import { remoteParent, remoteBasename, remoteJoin, normalizeRemotePathInput, localJoin } from './fileTree/paths';
  import { readDragPayload, isMultiDrag } from './fileTree/dragPayload';
  import { uniqueName } from './fileTree/uniqueName';
  import { describeDelete } from './fileTree/deletePrompt';
  import { loadPrefs, saveColumnPrefs as persistColumns, saveHiddenPref } from './fileTree/columnPrefs';
  import FilePaneHeader from './fileTree/FilePaneHeader.svelte';
  import './fileTree/fileTreeShared.css';
  import { ChevronUp } from 'lucide-svelte';

  const STORAGE_KEYS = { columns: 'filetree-show-columns', hidden: 'filetree-show-hidden' };

  export let sessionId: string;

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
  let permDialog = { show: false, path: '', isDir: false, mode: '' };
  let error = '';
  let ready = false;
  let osDropOff: (() => void) | null = null;
  let rootEl: HTMLDivElement | null = null;
  let dragOverPath: string | null = null;
  type SortDir = 'asc' | 'desc';
  let sortEnabled = false;
  let sortKey: SortKey | null = null;
  let sortDir: SortDir = 'asc';

  onMount(() => {
    ({ permissions: showPermissions, owner: showOwner, date: showDate } = loadPrefs(localStorage, STORAGE_KEYS).columns);
    if (rootEl) osDropOff = registerOsDropZone({ el: rootEl, onDrop: handleOsFileDrop });
  });

  // Recover SFTP readiness from the app-level latch. This is robust to the
  // one-shot SFTPReady event firing before this component mounted (fast warm
  // connections) or to a remount when the tab is dragged between tiles: the
  // store already holds the ready state and initial path for the session.
  $: if (!ready && $sftpReadyPaths.has(sessionId)) {
    ready = true;
    const initialPath = $sftpReadyPaths.get(sessionId);
    if (initialPath) currentPath = initialPath;
    refresh();
  }

  onDestroy(() => {
    if (osDropOff) osDropOff();
  });

  async function handleOsFileDrop(paths: string[], x: number, y: number) {
    if (!rootEl) return;
    const targetDir = resolveOsDropTarget(rootEl, x, y, currentPath);
    if (targetDir === null) return;
    await startUploadDrop(sessionId, paths, targetDir);
    await refreshPreservingState([targetDir, currentPath]);
  }

  async function loadDir(path: string) {
    if (loading.has(path)) return;
    loading.add(path);
    loading = loading;
    error = '';
    try {
      const nodes = await listPath(sessionId, path);
      rawTree.set(path, nodes);
      tree.set(path, applySort(nodes, sortState()));
      tree = tree;
    } catch (e: any) {
      error = e?.message || String(e);
    } finally {
      loading.delete(path);
      loading = loading;
    }
  }

  async function goUp() {
    const parent = remoteParent(currentPath);
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
    ({ selectedPaths, lastSelectedPath } = nextSelection(
      tree.get(currentPath) || [],
      { selectedPaths, lastSelectedPath },
      path,
      e
    ));
  }

  // This pane owns its selection only while the pointer keeps landing on its own
  // rows. Any other pointerdown — empty space here, the other pane, anywhere else
  // in the window — releases it (see paneSelection.ts).
  function dismissSelection(e: PointerEvent) {
    if (selectedPaths.size === 0) return;
    const target = e.target instanceof Element ? e.target : null;
    if (keepsPaneSelection(target, rootEl)) return;
    ({ selectedPaths, lastSelectedPath } = clearSelection());
  }

  async function navigateInto(path: string) {
    const node = findNodeIn(tree, path);
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

  // An operation that touched this session's remote tree finished (or was
  // partially applied — delete/chmod/chown mutate even on failure, which is why
  // subscribe.ts signals them on any terminal state). Which directories went
  // stale is answered entirely by refreshDir, the backend's machine-readable
  // path; remotePath is a caption ("3 items") and is never parsed.
  $: if (ready && $transferCompleted
      && refreshesRemotePane($transferCompleted.kind, $transferCompleted.sessionId, sessionId)) {
    const t = $transferCompleted;
    transferCompleted.set(null);
    refreshPreservingState([...remoteRefreshDirs(t.kind, t.refreshDir), currentPath]);
  }

  function sortState(): SortState {
    return { sortEnabled, sortKey, sortDir };
  }

  function toggleSort(nextKey: SortKey) {
    ({ sortEnabled, sortKey, sortDir } = cycleSortState(
      { sortEnabled, sortKey, sortDir },
      nextKey
    ));
    tree = sortTree(rawTree, sortState());
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
    const dt = e.dataTransfer;
    const { sessionId: dropSessionId, remotePaths, localPaths } = readDragPayload((k) => dt.getData(k));

    // A remote drag from this same session is a move within the host; one from
    // another session is not ours to rename, and falls through to the upload path.
    if (remotePaths.length > 0 && dropSessionId === sessionId) {
      const srcParents: string[] = [];
      for (const rp of remotePaths) {
        const destPath = remoteJoin(targetDir, remoteBasename(rp));
        if (!isInvalidMove(rp, targetDir)) {
          try {
            await renamePath(sessionId, rp, destPath);
            const srcParent = remoteParent(rp);
            if (!srcParents.includes(srcParent)) srcParents.push(srcParent);
          } catch (err: any) {
            error = err?.message || String(err);
          }
        }
      }
      if (srcParents.length > 0) await refreshPreservingState([targetDir, currentPath, ...srcParents]);
      return;
    }
    if (localPaths.length > 0) {
      await startUploadDrop(sessionId, localPaths, targetDir);
      await refreshPreservingState([targetDir, currentPath]);
    }
  }

  function handleDragStartFile(e: DragEvent, node: RemoteNode) {
    if (!e.dataTransfer) return;
    e.dataTransfer.effectAllowed = 'copy';
    const multi = isMultiDrag(selectedPaths, node.path);
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
    openContextMenu(closeContextMenu);
  }

  function closeContextMenu() {
    releaseContextMenu(closeContextMenu);
    ctxMenu = { ...ctxMenu, show: false };
  }

  async function handleKeydown(e: KeyboardEvent) {
    if (e.key === 'Delete' && selectedPaths.size > 0) {
      e.preventDefault();
      const paths = Array.from(selectedPaths);
      if (paths.length === 1) {
        const node = findNodeIn(tree, paths[0]);
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
    const name = remoteBasename(ctxMenu.path, ctxMenu.path);
    closeContextMenu();
    await requestDelete(ctxMenu.path, ctxMenu.isDir, name);
  }

  async function confirmDelete() {
    const { path, pathsToDelete } = deleteConfirm;
    deleteConfirm = { ...deleteConfirm, show: false };
    const toDelete = (pathsToDelete && pathsToDelete.length > 0) ? pathsToDelete : (path ? [path] : []);
    // Each delete runs as a background operation with progress in the Transfers
    // panel; the tree refreshes itself from the operation's terminal event
    // (see the $transferCompleted reactive block above). We only schedule here.
    for (const p of toDelete) {
      try {
        await removePath(sessionId, p);
      } catch (e: any) {
        error = e?.message || String(e);
      }
    }
    ({ selectedPaths, lastSelectedPath } = clearSelection());
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
      const localPath = localJoin(tempDir, remoteBasename(remotePath, 'file'));
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

  function namesIn(parentPath: string): string[] {
    return (tree.get(parentPath) || []).map((n) => n.name);
  }

  async function handleCtxNewFolder() {
    const parentPath = ctxMenu.isEmptyArea ? currentPath : ctxMenu.path;
    const baseName = uniqueName(namesIn(parentPath), 'New Folder');
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
    editingNewPath = remoteJoin(parentPath, baseName);
    tree = tree;
  }

  async function handlePathSubmit(typed: string) {
    const nextPath = normalizeRemotePathInput(typed);
    const prevPath = currentPath;
    // Fetch first: only commit navigation once the listing succeeds. A
    // non-existent directory must leave the current view untouched. listPath
    // normally swallows errors (returns []) and shows a global banner, which
    // would let navigation proceed into an empty non-existent folder — so we
    // opt into rethrow and surface the failure inline instead.
    try {
      const nodes = await listPath(sessionId, nextPath, { rethrow: true, silence: () => true });
      rawTree.set(nextPath, nodes);
      tree.set(nextPath, applySort(nodes, sortState()));
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
      header?.resetInput();
      return;
    }
  }

  let header: FilePaneHeader | null = null;

  function saveColumns() {
    persistColumns(localStorage, STORAGE_KEYS, { permissions: showPermissions, owner: showOwner, date: showDate });
  }

  function togglePermissions() { showPermissions = !showPermissions; saveColumns(); }
  function toggleOwner() { showOwner = !showOwner; saveColumns(); }
  function toggleDate() { showDate = !showDate; saveColumns(); }
  function toggleHidden() {
    showHidden = !showHidden;
    saveHiddenPref(localStorage, STORAGE_KEYS, showHidden);
    refreshPreservingState([...expanded, currentPath]);
  }

  async function handleCtxNewFile() {
    if (!ctxMenu.isDir) return;
    const parentPath = ctxMenu.path;
    const baseName = uniqueName(namesIn(parentPath), 'New File');
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

  function handleCtxPermissions() {
    if (ctxMenu.isEmptyArea || !ctxMenu.path) {
      closeContextMenu();
      return;
    }
    const node = findNodeIn(tree, ctxMenu.path);
    permDialog = { show: true, path: ctxMenu.path, isDir: ctxMenu.isDir, mode: node?.mode || '' };
    closeContextMenu();
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
    const parent = remoteParent(oldPath);
    const newPath = remoteJoin(parent, newName.trim());
    if (newPath === oldPath) {
      editingNewPath = null;
      return;
    }
    try {
      await renamePath(sessionId, oldPath, newPath);
    } catch (e: any) {
      error = e?.message || String(e);
      editingNewPath = null;
      return;
    }
    editingNewPath = null;
    await loadDir(parent);
    tree = tree;
  }

  function handleRenameCancel() {
    editingNewPath = null;
  }

  $: deletePrompt = describeDelete(deleteConfirm);

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
    refreshDisabled: !ready,
  });
</script>

<svelte:window on:click={closeContextMenu} on:pointerdown={dismissSelection} />
<div
  class="file-tree"
  class:internal-drop-active={internalDragHighlight(dragOverPath, currentPath) === 'pane'}
  bind:this={rootEl}
>
  <FilePaneHeader
    bind:this={header}
    title="Remote Files"
    {toolbarItems}
    {currentPath}
    {ready}
    {error}
    connectingLabel="Connecting SFTP..."
    on:navigate={(e) => handlePathSubmit(e.detail)}
    on:dismissError={() => (error = '')}
  />

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
    allowPermissionsMenu={true}
    on:delete={handleCtxDelete}
    on:newFolder={handleCtxNewFolder}
    on:newFile={handleCtxNewFile}
    on:rename={handleCtxRename}
    on:edit={handleCtxEdit}
    on:permissions={handleCtxPermissions}
  />

  <PermissionsDialog
    show={permDialog.show}
    {sessionId}
    path={permDialog.path}
    isDir={permDialog.isDir}
    currentMode={permDialog.mode}
    on:close={() => { permDialog = { ...permDialog, show: false }; refresh(); }}
  />

  <ConfirmDialog
    show={deleteConfirm.show}
    title={deletePrompt.title}
    message={deletePrompt.message}
    critical={deletePrompt.critical}
    requireCheckbox={deletePrompt.requireCheckbox}
    checkboxLabel="I understand"
    confirmLabel="Delete"
    on:confirm={confirmDelete}
    on:cancel={cancelDelete}
  />
</div>
