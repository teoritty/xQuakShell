// frontend/src/lib/tiles/geometry.test.ts
import { computeGrid } from './geometry';
import { computeResizers } from './resizers';
import { nextRatio, MIN_RATIO, MAX_RATIO } from './resizeMath';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error('FAIL: ' + msg);
}

const half = { main: 0.5, cross: 0.5 };

// n=1
const g1 = computeGrid(1, 'h', half);
assert(g1.areas.length === 1 && g1.columns === '1fr' && g1.rows === '1fr', 'n1 single cell');

// n=2 horizontal = two columns
const g2 = computeGrid(2, 'h', { main: 0.6, cross: 0.5 });
assert(g2.columns === '60fr 40fr' && g2.rows === '1fr', 'n2h columns from main');
assert(g2.areas[0] === '1 / 1 / 2 / 2' && g2.areas[1] === '1 / 2 / 2 / 3', 'n2h slot areas');

// n=2 vertical = two rows
const g2v = computeGrid(2, 'v', half);
assert(g2v.rows === '50fr 50fr' && g2v.columns === '1fr', 'n2v rows from main');

// n=3 h : full-left spans both rows
const g3 = computeGrid(3, 'h', half);
assert(g3.areas[0] === '1 / 1 / 3 / 2', 'n3h slot0 spans rows');
assert(g3.areas[1] === '1 / 2 / 2 / 3' && g3.areas[2] === '2 / 2 / 3 / 3', 'n3h right column');

// n=3 v : full-top spans both columns
const g3v = computeGrid(3, 'v', half);
assert(g3v.areas[0] === '1 / 1 / 2 / 3', 'n3v slot0 spans cols');

// n=4 : 2x2
const g4 = computeGrid(4, 'h', { main: 0.4, cross: 0.7 });
assert(g4.columns === '40fr 60fr' && g4.rows === '70fr 30fr', 'n4 grid tracks');
assert(g4.areas.length === 4 && g4.areas[3] === '2 / 2 / 3 / 3', 'n4 four cells');

// resizers
assert(computeResizers(1, 'h', half).length === 0, 'n1 no resizers');
assert(computeResizers(2, 'h', half).length === 1, 'n2 one resizer');
const r3 = computeResizers(3, 'h', { main: 0.5, cross: 0.5 });
assert(r3.length === 2 && r3[1].divider === 'cross' && r3[1].xPct === 50 && r3[1].wPct === 50, 'n3h cross bar spans right column');
assert(computeResizers(4, 'h', half).length === 2, 'n4 two resizers');

// resize math clamps
assert(nextRatio(0.5, 0, 100) === 0.5, 'no delta keeps ratio');
assert(nextRatio(0.5, -1000, 100) === MIN_RATIO, 'clamps to min');
assert(nextRatio(0.5, 1000, 100) === MAX_RATIO, 'clamps to max');
assert(nextRatio(0.5, 10, 100) === 0.6, 'proportional');

console.log('OK geometry');
