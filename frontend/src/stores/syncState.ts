// The sync status the indicator and the settings page show. A status event carries only how the
// last round went; the account fields come from GetSyncStatus and are kept across events.
import { writable } from 'svelte/store';
import type { SyncStatus } from '../backend/syncGateway';

export const offSyncStatus: SyncStatus = { enabled: false, running: false, state: 'off', held: 0, quarantined: 0 };

export const syncStatus = writable<SyncStatus>(offSyncStatus);

/** Merges one round's status into what is known about the account. */
export function applySyncRound(round: SyncStatus): void {
  syncStatus.update((cur) => ({
    ...cur,
    running: round.running,
    state: round.state,
    message: round.message,
    held: round.held,
    quarantined: round.quarantined,
    newDevices: round.newDevices,
  }));
}

/** A plugin data report worth the user's attention: files edited on two devices, or left out. */
export interface PluginDataNotice {
  pluginId: string;
  conflicts: string[];
  skipped: string[];
}

/** The newest notice per plugin, until the user dismisses it on the Sync page. */
export const pluginDataNotices = writable<PluginDataNotice[]>([]);

export function dismissPluginDataNotice(pluginId: string): void {
  pluginDataNotices.update((list) => list.filter((n) => n.pluginId !== pluginId));
}
