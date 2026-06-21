<script lang="ts">
  import Terminal from './Terminal.svelte';
  import FileTree from './FileTree.svelte';
  import LocalFileTree from './LocalFileTree.svelte';
  import TransferPanel from './TransferPanel.svelte';
  import type { Session } from '../stores/appState';
  import { closeSession, openSession, uploadFile, downloadFile } from '../stores/api';
  import { Loader2, XCircle, Circle } from 'lucide-svelte';

  export let session: Session;
  export let active: boolean = false;

  let splitRatio = 70;
  let isDragging = false;
  let fileSplitRatio = 50;
  let fileDragging = false;

  function startHResize(e: MouseEvent) {
    isDragging = true;
    const container = (e.target as HTMLElement).closest('.session-content') as HTMLElement;
    const startX = e.clientX;
    const startRatio = splitRatio;
    const containerWidth = container.getBoundingClientRect().width;

    function onMouseMove(ev: MouseEvent) {
      const delta = ev.clientX - startX;
      const newRatio = startRatio + (delta / containerWidth) * 100;
      splitRatio = Math.max(30, Math.min(85, newRatio));
    }

    function onMouseUp() {
      isDragging = false;
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
      window.dispatchEvent(new Event('resize'));
    }

    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
  }

  function startVResize(e: MouseEvent) {
    fileDragging = true;
    const container = (e.target as HTMLElement).closest('.files-column') as HTMLElement;
    const startY = e.clientY;
    const startRatio = fileSplitRatio;
    const containerHeight = container.getBoundingClientRect().height;

    function onMouseMove(ev: MouseEvent) {
      const delta = ev.clientY - startY;
      const newRatio = startRatio + (delta / containerHeight) * 100;
      fileSplitRatio = Math.max(20, Math.min(80, newRatio));
    }

    function onMouseUp() {
      fileDragging = false;
      window.removeEventListener('mousemove', onMouseMove);
      window.removeEventListener('mouseup', onMouseUp);
      window.dispatchEvent(new Event('resize'));
    }

    window.addEventListener('mousemove', onMouseMove);
    window.addEventListener('mouseup', onMouseUp);
  }

  function handleUpload(localPath: string, remotePath: string) {
    uploadFile(session.sessionId, localPath, remotePath);
  }

  function handleDownload(remotePath: string, sessionId: string, localDir: string) {
    downloadFile(sessionId, remotePath, localDir);
  }

  async function handleReconnect() {
    const connId = session.connectionId;
    await closeSession(session.sessionId);
    await openSession(connId);
  }
</script>

<div class="session-view" class:visible={active}>
  {#if session.state === 'connecting'}
    <div class="session-status">
      <div class="status-icon spinning"><Loader2 size={28} /></div>
      <div class="status-text">Connecting to {session.connectionName}...</div>
    </div>
  {:else if session.state === 'error'}
    <div class="session-status error">
      <div class="status-icon"><XCircle size={28} /></div>
      <div class="status-text">Connection error: {session.errorMessage}</div>
      <div class="status-actions">
        <button class="primary" on:click={handleReconnect}>Reconnect</button>
        <button class="secondary" on:click={() => closeSession(session.sessionId)}>Close</button>
      </div>
    </div>
  {:else if session.state === 'ready'}
    <div class="session-content" class:no-select={isDragging || fileDragging}>
      <div class="terminal-area" style="flex: {splitRatio}">
        {#key session.sessionId}
          <Terminal sessionId={session.sessionId} {active} />
        {/key}
      </div>
      <div
        class="split-handle-h"
        on:mousedown={startHResize}
        role="separator"
        aria-orientation="vertical"
      ></div>
      <div class="files-column" style="flex: {100 - splitRatio}">
        <div class="remote-files" style="flex: {fileSplitRatio}">
          <FileTree sessionId={session.sessionId} onDropUpload={handleUpload} />
        </div>
        <div
          class="split-handle-v"
          on:mousedown={startVResize}
          role="separator"
          aria-orientation="horizontal"
        ></div>
        <div class="local-files" style="flex: {100 - fileSplitRatio}">
          <LocalFileTree onDropDownload={handleDownload} />
        </div>
      </div>
    </div>
    <TransferPanel sessionId={session.sessionId} />
  {:else}
    <div class="session-status">
      <div class="status-icon"><Circle size={28} /></div>
      <div class="status-text">Session closed</div>
    </div>
  {/if}
</div>

<style>
  .session-view {
    display: none;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }

  .session-view.visible {
    display: flex;
  }

  .session-status {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 12px;
    flex: 1;
    padding: 32px;
  }

  .session-status.error {
    color: var(--danger);
  }

  .status-icon {
    color: var(--text-secondary);
  }

  .status-icon.spinning :global(svg) {
    animation: spin 1s linear infinite;
  }

  @keyframes spin {
    from { transform: rotate(0deg); }
    to { transform: rotate(360deg); }
  }

  .session-status.error .status-icon {
    color: var(--danger);
  }

  .status-text {
    font-size: 14px;
    color: var(--text-secondary);
    text-align: center;
    max-width: 400px;
  }

  .status-actions {
    display: flex;
    gap: 12px;
    margin-top: 8px;
  }

  .session-content {
    display: flex;
    flex-direction: row;
    flex: 1;
    min-height: 0;
    overflow: hidden;
  }

  .session-content.no-select {
    user-select: none;
  }

  .terminal-area {
    display: flex;
    flex-direction: column;
    min-width: 200px;
    min-height: 0;
    width: 100%;
    overflow: hidden;
  }

  .split-handle-h {
    width: 4px;
    background: var(--border-color);
    cursor: ew-resize;
    flex-shrink: 0;
    transition: background 0.15s;
  }

  .split-handle-h:hover { background: var(--accent); }

  .files-column {
    display: flex;
    flex-direction: column;
    min-width: 180px;
    min-height: 0;
    overflow: hidden;
    border-left: 1px solid var(--border-color);
  }

  .remote-files,
  .local-files {
    display: flex;
    flex-direction: column;
    min-height: 0;
    overflow: hidden;
  }

  .split-handle-v {
    height: 4px;
    background: var(--border-color);
    cursor: ns-resize;
    flex-shrink: 0;
    transition: background 0.15s;
  }

  .split-handle-v:hover { background: var(--accent); }
</style>
