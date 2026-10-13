// Sync status events. Their own module, registered beside subscribeToEvents rather than inside it,
// because that function is held to its recorded size.
import { getRuntime } from '../backend/context';
import type { SyncStatus } from '../backend/syncGateway';
import { applySyncRound } from '../stores/syncState';

export function subscribeSyncEvents(): void {
  const rt = getRuntime();
  if (!rt) return;
  rt.EventsOn('SyncStatus', (data: SyncStatus) => {
    if (data && typeof data.state === 'string') applySyncRound(data);
  });
}
