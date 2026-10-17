// What the Plugins panel of the Sync page shows for each offer and data notice (spec 6.4-6.5). Pure,
// so the rules - above all that nothing offers an automatic uninstall - are tested without a DOM.
import type { SyncPluginDataReport, SyncPluginOffer } from '../../../backend/syncGateway';
import type { PluginDataNotice } from '../../../stores/syncState';

export type OfferAction = 'install' | 'keep' | 'uninstall';

const ACTIONS: Record<SyncPluginOffer['state'], OfferAction[]> = {
  install: ['install'],
  update: ['install'],
  // A plugin installed from a file elsewhere has nothing to download; the message says so.
  manual: [],
  // The build here differs from the record: keep it as the new version, or install the recorded
  // one from the Plugins screen.
  mismatch: ['keep'],
  // Uninstalled on another device and disabled here. Uninstall deletes the plugin's data, so it is
  // a button the user presses, never something that happens on arrival.
  removed: ['keep', 'uninstall'],
};

export function offerActions(offer: SyncPluginOffer): OfferAction[] {
  return ACTIONS[offer.state] ?? [];
}

export function offerMessageKey(offer: SyncPluginOffer): string {
  return `sync.plugins.offer.${offer.state}`;
}

/** Keeps the newest report per plugin; a report with nothing to say adds nothing. */
export function mergePluginDataNotice(list: PluginDataNotice[], report: SyncPluginDataReport): PluginDataNotice[] {
  const conflicts = report.conflicts ?? [];
  const skipped = report.skipped ?? [];
  if (conflicts.length === 0 && skipped.length === 0) return list;
  return [...list.filter((n) => n.pluginId !== report.pluginId), { pluginId: report.pluginId, conflicts, skipped }];
}
