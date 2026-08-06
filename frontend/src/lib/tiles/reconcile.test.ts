// frontend/src/lib/tiles/reconcile.test.ts
import { emptyLayout } from './types';
import type { TileGroup, TileLayout } from './types';
import { reconcile } from './reconcile';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error('FAIL: ' + msg);
}
function tile(id: string, tabs: string[]): TileGroup {
  return { id, tabs: [...tabs], activeTabId: tabs[0] ?? '' };
}
function layout(tiles: TileGroup[]): TileLayout {
  return { tiles, orientation: 'h', activeTileId: tiles[0].id, dividers: { main: 0.5, cross: 0.5 } };
}

// New sessions land on the active tile
const l0 = emptyLayout();
const l1 = reconcile(l0, ['s1', 's2'], 's2');
assert(l1.tiles.length === 1 && l1.tiles[0].tabs.join() === 's1,s2', 'new sessions join active tile');
assert(l1.tiles[0].activeTabId === 's2', 'activeTabId focuses its tab');

// Closing a session removes its tab
const l2 = reconcile(l1, ['s1'], 's1');
assert(l2.tiles[0].tabs.join() === 's1', 'closed session tab removed');

// Emptying a tile collapses & down-normalizes 2 -> 1
const twoTiles = { ...layout([tile('A', ['s1']), tile('B', ['s2'])]), activeTileId: 'A' };
const collapsed = reconcile(twoTiles, ['s1'], 's1');
assert(collapsed.tiles.length === 1 && collapsed.tiles[0].id === 'A', '2->1 collapses emptied tile B');

// Active tile transfers when the active tile is the one that empties
const closeActive = reconcile({ ...twoTiles, activeTileId: 'B' }, ['s1'], 's1');
assert(closeActive.activeTileId === 'A', 'active transfers to survivor');

// Idempotent
const once = reconcile(twoTiles, ['s1', 's2'], 's2');
const twice = reconcile(once, ['s1', 's2'], 's2');
assert(JSON.stringify(once) === JSON.stringify(twice), 'reconcile is idempotent');

// New session goes to the *currently active* tile, not slot0
const l3 = reconcile({ ...twoTiles, activeTileId: 'B' }, ['s1', 's2', 's3'], 's3');
assert(l3.tiles.find((t) => t.id === 'B')!.tabs.join() === 's2,s3', 'new session joins active tile B');

// All sessions gone -> one empty tile (welcome screen)
const emptied = reconcile(twoTiles, [], '');
assert(emptied.tiles.length === 1 && emptied.tiles[0].tabs.length === 0, 'zero sessions keeps one empty tile');

// A plugin surface is an ordinary tab (ADR-015). The ids are opaque here, so the
// only thing worth pinning is that a surface id is neither stripped nor treated
// specially — the bug this replaces was reconcile never being told about it.
const withSurface = reconcile(layout([tile('A', ['s1'])]), ['s1', 'srf-1'], 'srf-1');
assert(withSurface.tiles[0].tabs.join() === 's1,srf-1', 'a surface joins the active tile like a session');
assert(withSurface.tiles[0].activeTabId === 'srf-1', 'a freshly opened surface takes focus');

// Closing the tab removes it and focus falls back to what is left.
const surfaceClosed = reconcile(withSurface, ['s1'], 's1');
assert(surfaceClosed.tiles[0].tabs.join() === 's1', 'a closed surface tab is stripped');
assert(surfaceClosed.tiles[0].activeTabId === 's1', 'focus returns to the surviving tab');

console.log('OK reconcile');
