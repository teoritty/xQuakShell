<script lang="ts">
  import { transfers, type TransferItem } from '../stores/appState';
  import { uploadFile, downloadFile, cancelTransfer, selectLocalFile, selectLocalDirectory } from '../stores/api';
  import { Upload, Download, ChevronDown, ChevronRight, X, RefreshCw } from 'lucide-svelte';

  export let sessionId: string;

  let collapsed = true;
  let prevCount = 0;
  let notifiedIds = new Set<string>();

  $: activeTransfers = $transfers.filter(t => t.sessionId === sessionId || !t.sessionId);

  $: {
    if (activeTransfers.length > prevCount && prevCount === 0) {
      collapsed = false;
    }
    prevCount = activeTransfers.length;
  }

  $: {
    for (const t of activeTransfers) {
      if (t.state === 'completed' && !notifiedIds.has(t.id)) {
        notifiedIds.add(t.id);
        notifiedIds = notifiedIds;
        try {
          if ('Notification' in window && Notification.permission === 'granted') {
            new Notification('Transfer completed', {
              body: `${t.direction === 'upload' ? 'Upload' : 'Download'}: ${t.remotePath}`,
            });
          } else if ('Notification' in window && Notification.permission !== 'denied') {
            Notification.requestPermission().then(p => {
              if (p === 'granted') {
                new Notification('Transfer completed', {
                  body: `${t.direction === 'upload' ? 'Upload' : 'Download'}: ${t.remotePath}`,
                });
              }
            });
          }
        } catch {}
      }
    }
  }

  $: hasActive = activeTransfers.length > 0;

  async function startUpload() {
    const localPath = await selectLocalFile();
    if (!localPath) return;
    const remotePath = prompt('Remote destination path:', '/tmp/' + localPath.split(/[\\/]/).pop());
    if (!remotePath) return;
    await uploadFile(sessionId, localPath, remotePath);
  }

  async function startDownload() {
    const remotePath = prompt('Remote file path to download:');
    if (!remotePath) return;
    const localDir = await selectLocalDirectory();
    if (!localDir) return;
    await downloadFile(sessionId, remotePath, localDir);
  }

  function progressPercent(item: TransferItem): number {
    if (item.total <= 0) return 0;
    return Math.round((item.done / item.total) * 100);
  }

  function isIndeterminate(item: TransferItem): boolean {
    return item.total <= 0 && item.state === 'active';
  }

  function getLocalDir(item: TransferItem): string {
    const p = item.localPath;
    const sep = p.includes('\\') ? '\\' : '/';
    const idx = p.lastIndexOf(sep);
    if (idx <= 0) return sep;
    return p.slice(0, idx) || sep;
  }

  async function retryTransfer(item: TransferItem) {
    if (!item.sessionId) return;
    if (item.direction === 'upload') {
      await uploadFile(item.sessionId, item.localPath, item.remotePath);
    } else {
      const localDir = getLocalDir(item);
      await downloadFile(item.sessionId, item.remotePath, localDir);
    }
  }

  function stateLabel(state: string): string {
    switch (state) {
      case 'pending': return 'Pending';
      case 'active': return 'Transferring';
      case 'completed': return 'Done';
      case 'failed': return 'Failed';
      case 'cancelled': return 'Cancelled';
      default: return state;
    }
  }
</script>

{#if hasActive}
  <div class="transfer-panel">
    <div
      class="panel-header clickable"
      on:click={() => collapsed = !collapsed}
      on:keydown={(e) => e.key === 'Enter' && (collapsed = !collapsed)}
      role="button"
      tabindex="0"
    >
      <span class="collapse-icon">
        {#if collapsed}<ChevronRight size={12} />{:else}<ChevronDown size={12} />{/if}
      </span>
      <span>Transfers ({activeTransfers.length})</span>
      <div class="actions" on:click|stopPropagation on:keydown|stopPropagation>
        <button on:click={startUpload} title="Upload file"><Upload size={11} /> Upload</button>
        <button on:click={startDownload} title="Download file"><Download size={11} /> Download</button>
      </div>
    </div>

    {#if !collapsed}
      <div class="transfer-list">
        {#each activeTransfers as item (item.id)}
          <div class="transfer-item" class:completed={item.state === 'completed'} class:failed={item.state === 'failed'} class:cancelled={item.state === 'cancelled'}>
            <div class="transfer-info">
              <span class="transfer-direction">
                {#if item.direction === 'upload'}<Upload size={11} />{:else}<Download size={11} />{/if}
              </span>
              <span class="transfer-path">{item.remotePath}</span>
              <span class="transfer-state">{stateLabel(item.state)}</span>
              {#if item.state === 'active' || item.state === 'pending'}
                <button class="cancel-btn" on:click={() => cancelTransfer(item.id)} title="Cancel"><X size={10} /></button>
              {:else if (item.state === 'failed' || item.state === 'cancelled') && item.sessionId}
                <button class="retry-btn" on:click={() => retryTransfer(item)} title="Retry"><RefreshCw size={10} /></button>
              {/if}
            </div>
            {#if item.state === 'active'}
              <div class="progress-bar" class:indeterminate={isIndeterminate(item)}>
                <div class="progress-fill" style="width: {isIndeterminate(item) ? 100 : progressPercent(item)}%"></div>
              </div>
              <div class="progress-text">{isIndeterminate(item) ? '…' : progressPercent(item) + '%'}</div>
            {/if}
          </div>
        {/each}
      </div>
    {/if}
  </div>
{/if}

<style>
  .transfer-panel {
    display: flex;
    flex-direction: column;
    border-top: 1px solid var(--border-color);
    background: var(--bg-primary);
  }

  .panel-header.clickable {
    cursor: pointer;
    display: flex;
    align-items: center;
    gap: 4px;
    padding: 4px 8px;
    font-size: 11px;
    font-weight: 600;
    background: var(--bg-secondary);
    border-bottom: 1px solid var(--border-color);
    user-select: none;
  }

  .collapse-icon {
    display: inline-flex;
    align-items: center;
    color: var(--text-secondary);
  }

  .actions {
    margin-left: auto;
    display: flex;
    gap: 4px;
  }

  .actions button {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    font-size: 10px;
    padding: 2px 6px;
  }

  .transfer-list {
    overflow-y: auto;
    max-height: 150px;
  }

  .transfer-item {
    padding: 4px 10px;
    border-bottom: 1px solid var(--border-color);
    font-size: 11px;
  }

  .transfer-item.completed { opacity: 0.6; }
  .transfer-item.failed,
  .transfer-item.cancelled { color: var(--danger); }

  .cancel-btn {
    background: none;
    border: none;
    color: var(--text-secondary);
    cursor: pointer;
    padding: 1px 3px;
    border-radius: 2px;
    display: inline-flex;
    align-items: center;
  }
  .cancel-btn:hover {
    color: var(--danger);
  }

  .retry-btn {
    background: none;
    border: none;
    color: var(--text-secondary);
    cursor: pointer;
    padding: 1px 3px;
    border-radius: 2px;
    display: inline-flex;
    align-items: center;
  }
  .retry-btn:hover {
    color: var(--accent);
  }

  .transfer-info {
    display: flex;
    align-items: center;
    gap: 6px;
  }

  .transfer-direction {
    display: inline-flex;
    align-items: center;
    flex-shrink: 0;
    color: var(--text-secondary);
  }

  .transfer-path {
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .transfer-state {
    font-size: 10px;
    color: var(--text-secondary);
    flex-shrink: 0;
  }

  .progress-bar {
    height: 3px;
    background: var(--bg-input);
    border-radius: 2px;
    margin-top: 3px;
    overflow: hidden;
  }

  .progress-fill {
    height: 100%;
    background: var(--accent);
    transition: width 0.2s;
  }

  .progress-bar.indeterminate .progress-fill {
    width: 30% !important;
    animation: indeterminate 1.5s ease-in-out infinite;
  }

  @keyframes indeterminate {
    0% { transform: translateX(-100%); }
    100% { transform: translateX(400%); }
  }

  .progress-text {
    font-size: 10px;
    color: var(--text-secondary);
    text-align: right;
    margin-top: 1px;
  }
</style>
