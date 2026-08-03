// frontend/src/lib/tiles/dropZones.test.ts
import { zoneAt } from './dropZones';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error('FAIL: ' + msg);
}

const rect = { left: 0, top: 0, width: 100, height: 100 };
const all = ['left', 'right', 'top', 'bottom'] as const;

assert(zoneAt(rect, 50, 50, [...all]) === 'center', 'middle = center');
assert(zoneAt(rect, 5, 50, [...all]) === 'left', 'near left edge = left');
assert(zoneAt(rect, 95, 50, [...all]) === 'right', 'near right edge = right');
assert(zoneAt(rect, 50, 5, [...all]) === 'top', 'near top edge = top');
assert(zoneAt(rect, 50, 95, [...all]) === 'bottom', 'near bottom = bottom');

// Disallowed edges collapse to center
assert(zoneAt(rect, 5, 50, ['top', 'bottom']) === 'center', 'left ignored when not allowed');
assert(zoneAt(rect, 5, 5, ['top', 'bottom']) === 'top', 'nearest allowed edge wins');

// Empty rect guards
assert(zoneAt({ left: 0, top: 0, width: 0, height: 0 }, 0, 0, [...all]) === 'center', 'zero-size = center');

console.log('OK dropZones');
