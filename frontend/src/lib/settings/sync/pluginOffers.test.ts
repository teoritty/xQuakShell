import { mergePluginDataNotice, offerActions, offerMessageKey } from './pluginOffers';
import type { SyncPluginOffer } from '../../../backend/syncGateway';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

const offer = (state: SyncPluginOffer['state']): SyncPluginOffer => ({ pluginId: 'p', version: '1', state });

// What the user can do about each state. Nothing ever offers an automatic uninstall, and a manual
// plugin has no install button because there is nothing to download.
const expected: Record<SyncPluginOffer['state'], string[]> = {
  install: ['install'],
  update: ['install'],
  manual: [],
  mismatch: ['keep'],
  removed: ['keep', 'uninstall'],
};
for (const [state, actions] of Object.entries(expected)) {
  const got = offerActions(offer(state as SyncPluginOffer['state']));
  assert(JSON.stringify(got) === JSON.stringify(actions), `${state}: actions ${got.join(',')}, want ${actions.join(',')}`);
  assert(offerMessageKey(offer(state as SyncPluginOffer['state'])) === `sync.plugins.offer.${state}`, `${state}: message key`);
}

// An unknown state from a newer backend shows no button rather than a wrong one.
assert(offerActions({ pluginId: 'p', version: '1', state: 'other' as never }).length === 0, 'an unknown state offers nothing');

// Notices: the newest report per plugin replaces the older one; an empty report adds nothing.
{
  let list = mergePluginDataNotice([], { pluginId: 'a', conflicts: ['x.json'], skipped: null });
  list = mergePluginDataNotice(list, { pluginId: 'b', conflicts: null, skipped: ['big.db'] });
  list = mergePluginDataNotice(list, { pluginId: 'a', conflicts: ['y.json'], skipped: [] });
  assert(list.length === 2, 'one notice per plugin');
  const a = list.find((n) => n.pluginId === 'a');
  assert(a?.conflicts.join() === 'y.json' && a.skipped.length === 0, 'the newer report replaces the older one');
  assert(mergePluginDataNotice(list, { pluginId: 'c', conflicts: [], skipped: null }).length === 2, 'an empty report adds no notice');
}

console.log('pluginOffers: all assertions passed');
