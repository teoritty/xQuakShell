// frontend/src/lib/tiles/operations.test.ts
import type { TileGroup, TileLayout } from './types';
import {
  splitOut,
  moveTab,
  reorient,
  splitEdges,
  reorientEdges,
  isLoneTab,
  tileOf,
} from './operations';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error('FAIL: ' + msg);
}
function tile(id: string, tabs: string[]): TileGroup {
  return { id, tabs: [...tabs], activeTabId: tabs[0] ?? '' };
}
function layout(tiles: TileGroup[], orientation: 'h' | 'v' = 'h'): TileLayout {
  return { tiles, orientation, activeTileId: tiles[0].id, dividers: { main: 0.5, cross: 0.5 } };
}

// --- isLoneTab ---
const oneTile = layout([tile('A', ['s1', 's2'])]);
assert(!isLoneTab(oneTile, 's1'), 'multi-tab tile is not lone');
assert(isLoneTab(layout([tile('A', ['s1'])]), 's1'), 'single-tab tile is lone');

// --- splitOut: only a multi-tab tile creates a new tile ---
const l1 = layout([tile('A', ['s1', 's2', 's3', 's4'])]);
const l2 = splitOut(l1, 's1', 'A', 'right');
assert(l2.tiles.length === 2 && l2.orientation === 'h', '1->2 makes 2 h tiles');
assert(l2.tiles[0].id === 'A' && l2.tiles[1].tabs.join() === 's1', '1->2 existing stays slot0, moved slot1');

const l2v = splitOut(l1, 's1', 'A', 'top');
assert(l2v.orientation === 'v' && l2v.tiles[0].tabs.join() === 's1', '1->2 top = vertical, moved slot0');

// A lone tab cannot split (no new tile from a single connection).
const lone = layout([tile('A', ['s1'])]);
assert(splitOut(lone, 's1', 'A', 'right') === lone, 'lone tab split is no-op');

// splitEdges at n=2 = perpendicular only (canonical T)
const twoH = l2; // A=[s2,s3,s4], B=[s1], orientation h
assert(splitEdges(twoH, 'A').join() === 'top,bottom', 'n2h split edges are perpendicular');

// 2 -> 3 : drop s2 on bottom of A (A subdivides; the untouched tile stays full slot0)
const untouched = twoH.tiles.find((t) => t.tabs.includes('s1'))!;
const l3 = splitOut(twoH, 's2', 'A', 'bottom');
assert(l3.tiles.length === 3, '2->3 makes 3 tiles');
assert(l3.tiles[0].id === untouched.id, '2->3 untouched tile becomes full slot0');
assert(tileOf(l3, 's2')!.tabs.join() === 's2', 's2 is now its own tile');

// n=3 split edges: only slot0 (full tile), cross axis
assert(splitEdges(l3, l3.tiles[1].id).length === 0, 'n3 non-full tile has no split edges');
assert(splitEdges(l3, l3.tiles[0].id).join() === 'top,bottom', 'n3 full tile splits on cross edges');

// 3 -> 4
const l3b = layout([tile('B', ['s1', 's9']), tile('C', ['s3']), tile('D', ['s4'])], 'h');
const l4 = splitOut(l3b, 's9', 'B', 'bottom');
assert(l4.tiles.length === 4, '3->4 makes 4 tiles');
assert(l4.tiles[0].id === 'B' && l4.tiles[2].tabs.join() === 's9', '3->4 h: [TL=B, TR, BL=moved, BR]');

// 4 -> no more splits
assert(splitOut(l4, 's3', l4.tiles[1].id, 'right') === l4, 'no 5th tile');
assert(splitEdges(l4, l4.tiles[0].id).length === 0, 'n4 has no split edges');

// --- reorient: lone tile dragged to an edge, count unchanged ---
assert(reorientEdges(layout([tile('A', ['s1'])])).length === 0, 'single tile has no reorient edges');
assert(reorientEdges(twoH).join() === 'left,right,top,bottom', 'n2 reorients on any edge');
assert(reorientEdges(l4).length === 0, 'n4 (2x2) has no reorient edges');

const sideBySide = layout([tile('A', ['s1']), tile('B', ['s2'])], 'h');
// cross-axis edge flips h -> v (stack), dragged tile on the dropped edge
const stacked = reorient(sideBySide, 's1', 'bottom');
assert(stacked.tiles.length === 2 && stacked.orientation === 'v', 'reorient flips h -> v');
assert(stacked.tiles[0].tabs.join() === 's2' && stacked.tiles[1].tabs.join() === 's1', 'dragged tile sits on bottom edge');
assert(stacked.tiles[1].id === 'A', 'reorient keeps tile ids (no remount)');

// same-axis edge swaps the two tiles' positions
const swapped = reorient(sideBySide, 's1', 'right');
assert(swapped.orientation === 'h' && swapped.tiles[0].id === 'B' && swapped.tiles[1].id === 'A', 'same-axis reorient swaps positions');

// n=3 reorient flips the T orientation on a cross-axis edge, no-op on same axis
const t3 = layout([tile('A', ['s1']), tile('B', ['s2']), tile('C', ['s3'])], 'h');
assert(reorient(t3, 's1', 'top').orientation === 'v', 'n3 reorient flips h -> v');
assert(reorient(t3, 's1', 'left') === t3, 'n3 same-axis reorient is no-op');

// --- moveTab: centre drop ---
const moved = moveTab(l4, 's1', l4.tiles[1].id);
assert(tileOf(moved, 's1')!.id === l4.tiles[1].id, 'moveTab relocates tab');

const two = layout([tile('A', ['s1']), tile('B', ['s2'])]);
const collapsed = moveTab(two, 's1', 'B');
assert(collapsed.tiles.length === 1 && collapsed.tiles[0].id === 'B', 'moveTab collapses emptied source');

console.log('OK operations');
