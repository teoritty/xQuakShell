import { writable } from 'svelte/store';
import { getRuntime } from '../backend/context';
import {
  folders, connections, sessions, identities,
  vaultUnlocked, transfers, transferCompleted, pendingHostKey,
  pingResults, editingFiles,
  type Session, type SessionEmbed,
  type TransferItem, type HostKeyEvent, type PingResult,
} from '../stores/appState';
import {
  appendPendingTerminalOutput,
  clearPendingTerminalOutput,
  clearTerminalOutputConsumer,
  decodeTerminalOutput,
  hasTerminalOutputConsumer,
} from '../terminal/outputBuffer';
import { disposeTerminal } from '../lib/terminalPool';
import { uploadFile } from '../api/remoteFs';
import { onDiscoveryTreeChanged } from '../stores/discoveryState';
import {
  upsertSurface,
  removeSurface,
  clearSurfaces,
  type Surface,
} from '../stores/surfaceState';
import {
  openDialog,
  closeDialog,
  setDialogError,
  activeDialog,
  type PluginDialog,
} from '../stores/dialogState';

// SFTPReady is a one-shot broadcast emitted once per session right after the
// remote filesystem is up. A FileTree component mounts only after its session
// reaches 'ready', so on fast (warm) connections the event can fire before the
// component subscribes — and a component that remounts (e.g. a tab dragged
// between tiles) would miss it too. We latch the readiness here at app-init,
// where a single always-on listener can never miss it, and expose it as a store
// so any (re)mounting FileTree can recover the session's ready state + initial
// path. Value = the session's initial remote path.
export const sftpReadyPaths = writable<Map<string, string>>(new Map());

export function subscribeToEvents(): void {
  const rt = getRuntime();
  if (!rt) return;

  rt.EventsOn('SFTPReady', (data: { sessionId: string; initialPath?: string }) => {
    if (!data?.sessionId) return;
    sftpReadyPaths.update(m => {
      const next = new Map(m);
      next.set(data.sessionId, data.initialPath || '/');
      return next;
    });
  });

  rt.EventsOn('SessionStateChanged', (data: Session) => {
    if (data.state === 'closed') {
      sftpReadyPaths.update(m => {
        if (!m.has(data.sessionId)) return m;
        const next = new Map(m);
        next.delete(data.sessionId);
        return next;
      });
      disposeTerminal(data.sessionId);
    }
    sessions.update(list => {
      if (data.state === 'closed') {
        return list.filter(s => s.sessionId !== data.sessionId);
      }
      const idx = list.findIndex(s => s.sessionId === data.sessionId);
      if (idx >= 0) {
        list[idx] = { ...list[idx], ...data };
        return [...list];
      }
      return [...list, data];
    });
  });

  rt.EventsOn('TerminalOutput', (data: { sessionId: string; output: string }) => {
    if (!data?.sessionId) return;
    if (hasTerminalOutputConsumer(data.sessionId)) return;
    appendPendingTerminalOutput(data.sessionId, decodeTerminalOutput(data.output));
  });

  // Plugin-owned tabs (ADR-015). Opened and Changed carry the whole surface, so both are one
  // upsert: an event that arrives twice leaves the store in the same place, which matters because
  // a plugin restarting republishes what it holds.
  rt.EventsOn('PluginSurfaceOpened', (data: Surface) => {
    if (!data?.surfaceId) return;
    upsertSurface(data);
  });

  rt.EventsOn('PluginSurfaceChanged', (data: Surface) => {
    if (!data?.surfaceId) return;
    upsertSurface(data);
  });

  rt.EventsOn('PluginSurfaceClosed', (data: { surfaceId: string }) => {
    if (!data?.surfaceId) return;
    removeSurface(data.surfaceId);
    // The pooled xterm instance and any buffered bytes go with it: a surface id is never reused,
    // so nothing else will ever come to collect them.
    disposeTerminal(data.surfaceId);
    clearPendingTerminalOutput(data.surfaceId);
  });

  // Buffered exactly like session output, and through the same buffer: a surface's tab may not be
  // mounted yet when its first bytes arrive, and dropping them would lose the start of a log.
  rt.EventsOn('PluginSurfaceOutput', (data: { surfaceId: string; data: string }) => {
    if (!data?.surfaceId) return;
    if (hasTerminalOutputConsumer(data.surfaceId)) return;
    appendPendingTerminalOutput(data.surfaceId, decodeTerminalOutput(data.data));
  });

  // Plugin dialogs (ADR-015). At most one is open at a time, which the host enforces, so the
  // frontend never holds a list.
  rt.EventsOn('PluginDialogOpened', (data: PluginDialog) => {
    if (!data?.dialogId) return;
    openDialog(data);
  });

  rt.EventsOn('PluginDialogClosed', (data: { dialogId: string }) => {
    if (!data?.dialogId) return;
    closeDialog(data.dialogId);
  });

  rt.EventsOn(
    'PluginDialogError',
    (data: { dialogId: string; message: string; fieldErrors: Record<string, string> }) => {
      if (!data?.dialogId) return;
      setDialogError(data.dialogId, data.message ?? '', data.fieldErrors ?? {});
    }
  );

  rt.EventsOn('SessionEmbedReady', (data: { sessionId: string; embed: SessionEmbed }) => {
    sessions.update(list => {
      const idx = list.findIndex(s => s.sessionId === data.sessionId);
      if (idx < 0) return list;
      list[idx] = {
        ...list[idx],
        surface: 'embed',
        embed: data.embed,
      };
      return [...list];
    });
  });

  rt.EventsOn('SessionClosed', (data: { sessionId: string }) => {
    clearPendingTerminalOutput(data.sessionId);
    clearTerminalOutputConsumer(data.sessionId);
    disposeTerminal(data.sessionId);
    sftpReadyPaths.update(m => {
      if (!m.has(data.sessionId)) return m;
      const next = new Map(m);
      next.delete(data.sessionId);
      return next;
    });
    sessions.update(list => list.filter(s => s.sessionId !== data.sessionId));
  });

  rt.EventsOn('TransferProgress', (data: TransferItem) => {
    // Byte transfers refresh trees only when they succeed; remote operations
    // (delete/chmod/chown) mutate the tree even on failure/cancel (partial
    // effect), so signal a refresh on any terminal state for those.
    const isOp = data.kind === 'delete' || data.kind === 'chmod' || data.kind === 'chown';
    const isTerminal = data.state === 'completed' || data.state === 'failed' || data.state === 'cancelled';
    const shouldRefresh = data.state === 'completed' || (isOp && isTerminal);
    transfers.update(list => {
      const idx = list.findIndex(t => t.id === data.id);
      if (idx >= 0) {
        list[idx] = { ...list[idx], ...data };
      } else {
        list = [...list, data];
      }
      if (shouldRefresh) {
        transferCompleted.set({ ...data });
      }
      return [...list];
    });
  });

  rt.EventsOn('HostKeyRequired', (data: HostKeyEvent) => {
    pendingHostKey.set(data);
  });

  rt.EventsOn('PingUpdated', (data: PingResult[]) => {
    const map = new Map<string, PingResult>();
    if (Array.isArray(data)) {
      for (const r of data) map.set(r.connectionId, r);
    }
    pingResults.set(map);
  });

  // ADR-014. The payload names the changed node, but the read side is a
  // whole-connection snapshot, so the store refetches by connectionId and
  // ignores nodeId beyond using it as the "something moved" signal. The backend
  // already coalesces these at 100 ms per node.
  rt.EventsOn('DiscoveryTreeChanged', (data: { connectionId: string; nodeId?: string }) => {
    onDiscoveryTreeChanged(data?.connectionId ?? '');
  });

  rt.EventsOn('VaultLocked', () => {
    vaultUnlocked.set(false);
    folders.set([]);
    connections.set([]);
    sessions.set([]);
    identities.set([]);
    // Surfaces and dialogs belong to sessions that no longer exist; leaving them on screen would
    // show a plugin's tabs and modals over a locked vault.
    clearSurfaces();
    activeDialog.set(null);
  });

  rt.EventsOn('FileEdited', (data: { localPath: string }) => {
    const path = data?.localPath;
    if (!path) return;
    editingFiles.update((map) => {
      const entry = map.get(path);
      if (entry) {
        uploadFile(entry.sessionId, path, entry.remotePath);
        const next = new Map(map);
        next.delete(path);
        return next;
      }
      return map;
    });
  });
}
