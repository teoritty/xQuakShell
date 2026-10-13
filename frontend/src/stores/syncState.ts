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
