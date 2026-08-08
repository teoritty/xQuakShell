import { clampSidebarWidth, MIN_SIDEBAR_WIDTH } from './sidebarWidth';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

const WIDE = 1920;

// The starting width is also the floor: dragging left must not shave a pixel off it.
assert(clampSidebarWidth(MIN_SIDEBAR_WIDTH, WIDE) === 280, 'the starting width survives a clamp');
assert(clampSidebarWidth(279, WIDE) === 280, 'one pixel under the floor is refused');
assert(clampSidebarWidth(0, WIDE) === 280, 'dragging past the left edge stops at the floor');
assert(clampSidebarWidth(-500, WIDE) === 280, 'a negative width stops at the floor');

// Widening is the whole point of the feature.
assert(clampSidebarWidth(281, WIDE) === 281, 'one pixel over the floor is allowed');
assert(clampSidebarWidth(420, WIDE) === 420, 'a width between the bounds is taken as given');
assert(clampSidebarWidth(640, WIDE) === 640, 'the ceiling itself is allowed');
assert(clampSidebarWidth(641, WIDE) === 640, 'one pixel over the ceiling is refused');
assert(clampSidebarWidth(99999, WIDE) === 640, 'dragging past the right edge stops at the ceiling');

// Half the viewport, so the terminal cannot be dragged out of existence on a small window.
assert(clampSidebarWidth(640, 1000) === 500, 'the ceiling is half the viewport when that is smaller');
assert(clampSidebarWidth(400, 1000) === 400, 'a width under half the viewport is untouched');
assert(clampSidebarWidth(500, 900) === 450, 'the half-viewport ceiling tracks the window');

// Below twice the minimum the two bounds cross, and the floor has to win: the alternative is a
// sidebar narrower than the width it is documented never to go below.
assert(clampSidebarWidth(400, 500) === 280, 'the floor beats a half-viewport ceiling under it');
assert(clampSidebarWidth(280, 100) === 280, 'a viewport narrower than the sidebar still yields the floor');

// A fractional drag delta must not reach the DOM as a fractional pixel.
assert(clampSidebarWidth(400.4, WIDE) === 400, 'a fractional width is rounded down');
assert(clampSidebarWidth(400.6, WIDE) === 401, 'a fractional width is rounded up');

// Guards against arithmetic on a missing measurement rather than trusting the caller.
assert(clampSidebarWidth(NaN, WIDE) === 280, 'NaN falls back to the floor');
assert(clampSidebarWidth(Infinity, WIDE) === 640, 'an infinite width is still capped');
assert(clampSidebarWidth(400, NaN) === 400, 'an unmeasurable viewport falls back to the fixed ceiling');

console.log('sidebarWidth.test.ts: all passed');
