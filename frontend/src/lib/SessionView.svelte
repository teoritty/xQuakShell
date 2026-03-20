<script lang="ts">
  import { onMount } from 'svelte';
  import Terminal from './Terminal.svelte';
  import FileTree from './FileTree.svelte';
  import LocalFileTree from './LocalFileTree.svelte';
  import TransferPanel from './TransferPanel.svelte';
  import type { Session } from '../stores/appState';
  import { connections, platform } from '../stores/appState';
  import { closeSession, openSession, uploadFile, downloadFile, rdpStart, rdpStop } from '../stores/api';
  import { Loader2, XCircle, Circle, Monitor, ExternalLink, Focus, RefreshCw } from 'lucide-svelte';

  export let session: Session;
  export let active: boolean = false;

  $: httpConn = $connections.find(c => c.id === session.connectionId);
  $: httpEmbedUrl = (() => {
    const u = httpConn?.httpConfig?.url?.trim() || '';
    if (!u) return '';
    if (/^https?:\/\//i.test(u)) return u;
    return `https://${u}`;
  })();

  let splitRatio = 70;
  let isDragging = false;
  let fileSplitRatio = 50;
  let fileDragging = false;

  let rdpLaunched = false;
  let rdpError = '';
  let rdpLaunching = false;
  let suppressAutoLaunch = false;

  async function launchRDP() {
    if (rdpLaunching) return;
    rdpLaunching = true;
    rdpError = '';
    try {
      const mode = await rdpStart(session.sessionId);
      if (mode) {
        rdpLaunched = true;
      } else {
        rdpError = 'Failed to launch RDP client';
      }
    } catch (e: any) {
      rdpError = e?.message || String(e);
    } finally {
      rdpLaunching = false;
    }
  }

  async function focusRDP() {
    // Ensure window is brought up; backend will focus existing process
    // or start a new one when no tracked process exists.
    await launchRDP();
  }

  async function stopRDP() {
    suppressAutoLaunch = true;
    await rdpStop(session.sessionId);
    rdpLaunched = false;
  }

  async function reconnectRDP() {
    suppressAutoLaunch = true;
    await stopRDP();
    await launchRDP();
    suppressAutoLaunch = false;
  }

  async function closeRDPAndSession() {
    suppressAutoLaunch = true;
    await stopRDP();
    await closeSession(session.sessionId);
  }

  $: isRDP = (session.protocol || 'ssh') === 'rdp';

  $: if (isRDP && session.state === 'ready' && !rdpLaunched && !rdpLaunching && !suppressAutoLaunch) {
    launchRDP();
  }

  onMount(() => {
    if (isRDP && session.state === 'ready' && !rdpLaunched && !rdpLaunching && !suppressAutoLaunch) {
      launchRDP();
    }
  });

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
    {#if (session.protocol || 'ssh') === 'http' && httpEmbedUrl}
      <div class="http-embed-wrap">
        <iframe
          class="http-iframe"
          title="HTTP session"
          src={httpEmbedUrl}
          sandbox="allow-scripts allow-same-origin allow-forms allow-popups allow-modals allow-downloads"
        ></iframe>
        <p class="http-hint">
          If the page is empty, the site may block embedding (X-Frame-Options / CSP). Open the URL in an external browser from the connection card.
        </p>
        <div class="session-actions-bar">
          <button class="secondary" on:click={() => closeSession(session.sessionId)}>Close</button>
        </div>
      </div>
    {:else if (session.protocol || 'ssh') === 'http'}
      <div class="session-status">
        <div class="status-icon"><XCircle size={28} /></div>
        <div class="status-text">No URL configured for HTTP session in connection settings.</div>
        <div class="status-actions">
          <button class="secondary" on:click={() => closeSession(session.sessionId)}>Close</button>
        </div>
      </div>
    {:else if (session.protocol || 'ssh') === 'rdp'}
      <div class="rdp-native-wrap">
        <div class="rdp-native-status">
          <div class="rdp-icon"><Monitor size={48} strokeWidth={1.5} /></div>
          {#if rdpLaunched}
            <div class="rdp-title">RDP session opened in native window</div>
            <div class="rdp-hint">
              {$platform === 'windows' ? 'mstsc.exe' : 'xfreerdp'} is running in a separate window. Use the buttons below to manage the session.
            </div>
          {:else if rdpError}
            <div class="rdp-title rdp-error-text">Failed to launch RDP client</div>
            <div class="rdp-hint rdp-error-text">{rdpError}</div>
          {:else}
            <div class="rdp-title">Starting RDP session...</div>
          {/if}
        </div>
        <div class="rdp-actions">
          {#if rdpLaunched}
            <button class="primary" on:click={focusRDP}>
              <Focus size={16} /> Focus window
            </button>
            <button class="secondary" on:click={reconnectRDP}>
              <RefreshCw size={16} /> Reconnect
            </button>
          {:else}
            <button class="primary" on:click={launchRDP}>
              <ExternalLink size={16} /> Open RDP
            </button>
          {/if}
          <button class="secondary danger-btn" on:click={closeRDPAndSession}>
            <XCircle size={16} /> Close
          </button>
        </div>
      </div>
    {:else}
      <div class="session-content" class:no-select={isDragging || fileDragging}>
        <div class="terminal-area" style="flex: {splitRatio}">
          {#key session.sessionId}
            <Terminal sessionId={session.sessionId} {active} />
          {/key}
        </div>
        {#if (session.protocol || 'ssh') === 'ssh'}
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
        {:else}
          <div
            class="split-handle-h"
            on:mousedown={startHResize}
            role="separator"
            aria-orientation="vertical"
          ></div>
          <div class="files-column" style="flex: {100 - splitRatio}">
            <div class="local-files" style="flex: 1">
              <LocalFileTree onDropDownload={handleDownload} />
            </div>
          </div>
        {/if}
      </div>
      {#if (session.protocol || 'ssh') === 'ssh'}
        <TransferPanel sessionId={session.sessionId} />
      {/if}
    {/if}
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

  .http-embed-wrap {
    display: flex;
    flex-direction: column;
    flex: 1;
    min-height: 0;
    padding: 8px;
    gap: 8px;
  }

  .http-iframe {
    flex: 1;
    min-height: 200px;
    width: 100%;
    border: 1px solid var(--border-color);
    border-radius: 6px;
    background: #fff;
  }

  .http-hint {
    font-size: 12px;
    color: var(--text-secondary);
    margin: 0;
    line-height: 1.4;
  }

  .rdp-native-wrap {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    flex: 1;
    min-height: 0;
    padding: 32px;
    gap: 24px;
  }

  .rdp-native-status {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    text-align: center;
  }

  .rdp-icon {
    color: var(--accent, #4fc3f7);
    opacity: 0.8;
  }

  .rdp-title {
    font-size: 16px;
    font-weight: 600;
    color: var(--text-primary);
  }

  .rdp-hint {
    font-size: 13px;
    color: var(--text-secondary);
    max-width: 400px;
    line-height: 1.5;
  }

  .rdp-error-text {
    color: var(--danger, #d32f2f);
  }

  .rdp-actions {
    display: flex;
    gap: 12px;
    flex-wrap: wrap;
    justify-content: center;
  }

  .rdp-actions button {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }

  .danger-btn {
    color: var(--danger, #d32f2f) !important;
    border-color: var(--danger, #d32f2f) !important;
  }

  .session-actions-bar {
    display: flex;
    justify-content: flex-end;
  }
</style>
