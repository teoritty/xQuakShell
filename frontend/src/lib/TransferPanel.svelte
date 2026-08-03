<script lang="ts">
  import { transfers, clearFinishedTransfers, type TransferItem, type OperationKind } from '../stores/appState';
  import { uploadFile, downloadFile, cancelTransfer } from '../api/remoteFs';
  import { selectLocalFile, selectLocalDirectory } from '../api/localFs';
  import { Upload, Download, ChevronDown, ChevronRight, X, RefreshCw, Trash2, Lock, User } from 'lucide-svelte';
  import type { ComponentType } from 'svelte';
  import { onMount } from 'svelte';
  import { createRateTracker } from './transferRate';
  import { formatBytesPerSec } from './formatBytes';
  import { kindLabel, showsRate, isScanning as isScanningState } from './transferPresentation';

  const KIND_ICON: Record<OperationKind, ComponentType> = {
    upload: Upload,
    download: Download,
    localcopy: Upload, // a local copy places files, same as an upload — reuse the icon
    delete: Trash2,
    chmod: Lock,
    chown: User,
  };

  function kindIcon(kind: OperationKind): ComponentType {
    return KIND_ICON[kind] ?? Upload;
  }

  // An operation is still scanning when it is active and no total is known
  // yet; show a live scan counter instead of a percentage/indeterminate bar.
  // This holds regardless of kind — see transferPresentation.ts.
  function isScanning(item: TransferItem): boolean {
    return isScanningState(item.state, item.total);
  }

  function progressText(item: TransferItem): string {
    if (isScanning(item)) return item.done > 0 ? `Scanning ${item.done}…` : 'Scanning…';
    return progressPercent(item) + '%';
  }

  export let sessionId: string;

  let collapsed = true;
  // When true the panel is fully hidden (dismissed via the close button), as
  // opposed to `collapsed` which only folds the list away. A new batch of
  // transfers clears it so the panel comes back.
  let dismissed = false;
  let prevCount = 0;
  let notifiedIds = new Set<string>();

  $: activeTransfers = $transfers.filter(t => t.sessionId === sessionId || !t.sessionId);

  // Byte-rate estimation lives in the presentation layer (see transferRate.ts).
  // Only byte transfers (upload/download/localcopy) have a meaningful rate;
  // remote ops (delete/chmod/chown) and scanning do not — see showsRate in
  // transferPresentation.ts.
  //
  // Sampling is driven by a fixed tick rather than the progress-event stream:
  // events arrive many times per second, which made the displayed speed
  // flicker unreadably. Refreshing twice per second keeps the number legible
  // while the EMA in the tracker smooths short bursts.
  const SPEED_REFRESH_MS = 500;
  const rateTracker = createRateTracker();
  let speeds: Record<string, string> = {};
  let sampledIds = new Set<string>();

  onMount(() => {
    const iv = setInterval(refreshSpeeds, SPEED_REFRESH_MS);
    return () => clearInterval(iv);
  });

  function refreshSpeeds() {
    const now = Date.now();
    const next: Record<string, string> = {};
    const active = new Set<string>();
    for (const t of activeTransfers) {
      if (showsRate(t.kind, t.state)) {
        active.add(t.id);
        const text = formatBytesPerSec(rateTracker.sample(t.id, t.done, now));
        if (text) next[t.id] = text;
      }
    }
    // Release tracker state for transfers that are no longer active.
    for (const id of sampledIds) {
      if (!active.has(id)) rateTracker.clear(id);
    }
    sampledIds = active;
    speeds = next;
  }

  $: {
    // Completed transfers stay in the list, so the count never returns to 0
    // within a session. Detect a *new* operation by the list growing, and
    // use that both to re-show a dismissed panel and to auto-open on the
    // first batch.
    if (activeTransfers.length > prevCount) {
      dismissed = false;
      if (prevCount === 0) collapsed = false;
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
            new Notification('Operation completed', {
              body: `${kindLabel(t.kind)}: ${t.remotePath}`,
            });
          } else if ('Notification' in window && Notification.permission !== 'denied') {
            Notification.requestPermission().then(p => {
              if (p === 'granted') {
                new Notification('Transfer completed', {
                  body: `${kindLabel(t.kind)}: ${t.remotePath}`,
                });
              }
            });
          }
        } catch {}
      }
    }
  }

  $: hasActive = activeTransfers.length > 0;

  // Closing the panel clears finished history (keeping any in-progress items)
  // and folds the panel away. prevCount is realigned so the grow-detector above
  // doesn't read the shrink as a brand-new batch and re-open.
  function closePanel() {
    clearFinishedTransfers();
    dismissed = true;
    prevCount = $transfers.filter(t => t.sessionId === sessionId || !t.sessionId).length;
  }

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

  function getLocalDir(item: TransferItem): string {
    const p = item.localPath;
    const sep = p.includes('\\') ? '\\' : '/';
    const idx = p.lastIndexOf(sep);
    if (idx <= 0) return sep;
    return p.slice(0, idx) || sep;
  }

  // Retry re-issues the original single-path call, so it needs both real paths.
  // Only the single-path transfer API fills localPath; a planned batch leaves it
  // empty and puts a caption ("3 items") in remotePath, which must never be sent
  // back as a path. canRetry gates the button on exactly that.
  function canRetry(item: TransferItem): boolean {
    return !!item.sessionId && !!item.localPath
      && (item.kind === 'upload' || item.kind === 'download')
      && (item.state === 'failed' || item.state === 'cancelled');
  }

  async function retryTransfer(item: TransferItem) {
    if (!canRetry(item)) return;
    if (item.kind === 'upload') {
      await uploadFile(item.sessionId, item.localPath, item.remotePath);
    } else {
      const localDir = getLocalDir(item);
      await downloadFile(item.sessionId, item.remotePath, localDir);
    }
  }

  function stateLabel(item: TransferItem): string {
    switch (item.state) {
      case 'pending': return 'Pending';
      case 'active':
        switch (item.kind) {
          case 'delete': return 'Deleting';
          case 'chmod':
          case 'chown': return 'Applying';
          default: return 'Transferring';
        }
      case 'completed': return 'Done';
      case 'failed': return 'Failed';
      case 'cancelled': return 'Cancelled';
      default: return item.state;
    }
  }
</script>

{#if hasActive && !dismissed}
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
        <!-- <button on:click={startUpload} title="Upload file"><Upload size={11} /> Upload</button>
        <button on:click={startDownload} title="Download file"><Download size={11} /> Download</button> -->
        <button class="cancel-btn" on:click={closePanel} title="Close"><X size={13} /></button>
      </div>
    </div>

    {#if !collapsed}
      <div class="transfer-list">
        {#each activeTransfers as item (item.id)}
          <div class="transfer-item" class:completed={item.state === 'completed'} class:failed={item.state === 'failed'} class:cancelled={item.state === 'cancelled'}>
            <div class="transfer-info">
              <span class="transfer-direction" title={kindLabel(item.kind)}>
                <svelte:component this={kindIcon(item.kind)} size={11} />
              </span>
              <span class="transfer-path">{item.remotePath}</span>
              <span class="transfer-state">{stateLabel(item)}</span>
              {#if item.state === 'active' || item.state === 'pending'}
                <button class="cancel-btn" on:click={() => cancelTransfer(item.id)} title="Cancel"><X size={10} /></button>
              {:else if canRetry(item)}
                <button class="retry-btn" on:click={() => retryTransfer(item)} title="Retry"><RefreshCw size={10} /></button>
              {/if}
            </div>
            {#if item.state === 'active'}
              <div class="progress-bar" class:indeterminate={isScanning(item)}>
                <div class="progress-fill" style="width: {isScanning(item) ? 100 : progressPercent(item)}%"></div>
              </div>
              <div class="progress-text">
                {#if speeds[item.id]}<span class="progress-speed">{speeds[item.id]}</span>{/if}
                <span>{progressText(item)}</span>
              </div>
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
    display: flex;
    justify-content: flex-end;
    align-items: baseline;
    margin-top: 1px;
  }

  /* Speed sits to the left of the percent; margin-right:auto keeps the
     percent pinned right whether or not a speed is shown. */
  .progress-speed {
    margin-right: auto;
    font-variant-numeric: tabular-nums;
  }
</style>
