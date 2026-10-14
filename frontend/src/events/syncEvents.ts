// Sync status events. Their own module, registered beside subscribeToEvents rather than inside it,
// because that function is held to its recorded size.
import { getRuntime } from '../backend/context';
import type { SyncPluginDataReport, SyncStatus } from '../backend/syncGateway';
import { applySyncRound, pluginDataNotices } from '../stores/syncState';
import { mergePluginDataNotice } from '../lib/settings/sync/pluginOffers';

export function subscribeSyncEvents(): void {
  const rt = getRuntime();
  if (!rt) return;
  rt.EventsOn('SyncStatus', (data: SyncStatus) => {
    if (data && typeof data.state === 'string') applySyncRound(data);
  });
  // A plugin's files were edited on two devices (the local edit won, the other is in the trash) or
  // were left out of sync. Kept until dismissed, because the user may not be looking right now.
  rt.EventsOn('SyncPluginData', (data: SyncPluginDataReport) => {
    if (data && typeof data.pluginId === 'string') pluginDataNotices.update((l) => mergePluginDataNotice(l, data));
  });
}
