import { clampPanelHeight, MIN_PANEL_HEIGHT } from './sidebarPanelHeight';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

// 1000px of window: the 80% ceiling is 800, the leave-the-tree-alone ceiling is 820, so 800 wins.
const TALL = 1000;

// The floor is what stops a drag upward from reducing the panel to its own header.
assert(clampPanelHeight(MIN_PANEL_HEIGHT, TALL) === 140, 'the floor survives a clamp');
assert(clampPanelHeight(139, TALL) === 140, 'one pixel under the floor is refused');
assert(clampPanelHeight(0, TALL) === 140, 'dragging past the top stops at the floor');
assert(clampPanelHeight(-500, TALL) === 140, 'a negative height stops at the floor');

// Growing the panel is the point of the feature.
assert(clampPanelHeight(141, TALL) === 141, 'one pixel over the floor is allowed');
assert(clampPanelHeight(420, TALL) === 420, 'a height between the bounds is taken as given');

// Two ceilings, and the lower of them applies. At 1000px that is the 80% fraction.
assert(clampPanelHeight(800, TALL) === 800, 'the ceiling itself is allowed');
assert(clampPanelHeight(801, TALL) === 800, 'one pixel over the ceiling is refused');
assert(clampPanelHeight(99999, TALL) === 800, 'dragging past the bottom stops at the ceiling');

// On a tall window the tree reservation is the binding constraint rather than the fraction:
// 80% of 2000 is 1600, but leaving the tree 180px allows 1820, so the fraction still wins. At a
// height where the two swap, the smaller must be the one that applies.
assert(clampPanelHeight(99999, 2000) === 1600, 'the fraction caps a tall window');
assert(clampPanelHeight(99999, 400) === 220, 'the tree keeps its 180px when that is the tighter bound');

// Below the point where the bounds cross, the floor wins: a panel under its own minimum is
// unusable, while a squeezed tree is merely uncomfortable.
assert(clampPanelHeight(300, 300) === 140, 'the floor beats a tree-reservation ceiling under it');
assert(clampPanelHeight(140, 100) === 140, 'a window shorter than the panel still yields the floor');

// A fractional drag delta must not reach the DOM as a fractional pixel.
assert(clampPanelHeight(400.4, TALL) === 400, 'a fractional height is rounded down');
assert(clampPanelHeight(400.6, TALL) === 401, 'a fractional height is rounded up');

// Guards against arithmetic on a missing measurement rather than trusting the caller.
assert(clampPanelHeight(NaN, TALL) === 140, 'NaN falls back to the floor');
assert(clampPanelHeight(Infinity, TALL) === 800, 'an infinite height is still capped');
assert(clampPanelHeight(400, NaN) === 140, 'an unmeasurable viewport falls back to the floor');

console.log('sidebarPanelHeight.test.ts: all passed');
