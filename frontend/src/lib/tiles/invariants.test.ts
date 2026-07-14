// frontend/src/lib/tiles/invariants.test.ts
import { emptyLayout, newTileId } from './types';
import type { TileGroup, TileLayout } from './types';
import { removeTab, fixActiveTab, withTiles } from './invariants';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error('FAIL: ' + msg);
}

function tile(id: string, tabs: string[], active = tabs[0] ?? ''): TileGroup {
  return { id, tabs: [...tabs], activeTabId: active };
}

// removeTab: drops the tab and repoints activeTabId to the last remaining
const t = removeTab(tile('a', ['s1', 's2', 's3'], 's2'), 's2');
assert(t.tabs.join(',') === 's1,s3', 'removeTab keeps order');
assert(t.activeTabId === 's3', 'removeTab repoints active to last');

// removeTab: removing a non-active tab leaves active untouched
const t2 = removeTab(tile('a', ['s1', 's2'], 's1'), 's2');
assert(t2.activeTabId === 's1', 'removeTab keeps unaffected active');

// fixActiveTab: dangling active is repointed
assert(fixActiveTab(tile('a', ['s1'], 'gone')).activeTabId === 's1', 'fixActiveTab repoints');

// withTiles: same count keeps dividers; different count resets them
const base: TileLayout = { ...emptyLayout(), dividers: { main: 0.7, cross: 0.3 } };
const twoTiles = [tile('x', ['s1']), tile('y', ['s2'])];
const grown = withTiles(base, twoTiles);
assert(grown.dividers.main === 0.5 && grown.dividers.cross === 0.5, 'withTiles resets dividers on count change');

const sameCount = withTiles({ ...base, tiles: [tile('x', ['s1'])] }, [tile('x', ['s1', 's2'])]);
assert(sameCount.dividers.main === 0.7, 'withTiles keeps dividers when count stable');

// withTiles: activeTileId falls back to slot 0 when the old active tile is gone
const orphan = withTiles({ ...base, activeTileId: 'gone' }, twoTiles);
assert(orphan.activeTileId === 'x', 'withTiles falls back active tile to slot0');

// unique ids
assert(newTileId() !== newTileId(), 'newTileId unique');

console.log('OK invariants');
