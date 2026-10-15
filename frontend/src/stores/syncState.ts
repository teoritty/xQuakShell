// The sync status the indicator and the settings page show. A status event carries how the last
// round went and, when sync is enabled, the account's side too; the fields an event leaves out keep
// what GetSyncStatus last said.
import { writable } from 'svelte/store';
import type { SyncStatus } from '../backend/syncGateway';
import type { CredentialBinding } from '../backend/teamGateway';

export const offSyncStatus: SyncStatus = { enabled: false, running: false, state: 'off', held: 0, quarantined: 0 };

export const syncStatus = writable<SyncStatus>(offSyncStatus);

/**
 * Bumped whenever sync changed the vault - a pull applied something, or a sync action here wrote to
 * it. Every view that lists vault content re-reads on it: without it an open screen showed the
 * vault as it was when the screen opened, and a form saved from such a screen reverted what another
 * device had changed.
 */
export const syncRevision = writable(0);

/** This member's links from team connection users to credentials of their own vault. */
export const credentialBindings = writable<CredentialBinding[]>([]);

/** Merges one round's status into what is known about the account. */
export function applySyncRound(round: SyncStatus): void {
  syncStatus.update((cur) => ({
    ...cur,
    running: round.running,
    state: round.state,
    message: round.message,
    stopReason: round.stopReason,
    held: round.held,
    quarantined: round.quarantined,
    newDevices: round.enabled ? round.newDevices : cur.newDevices,
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
