// frontend/src/lib/tiles/dragPayload.test.ts
import { writeDragPayload, readDragPayload, isTileTabDrag, TILE_TAB_MIME } from './dragPayload';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error('FAIL: ' + msg);
}

// Minimal DataTransfer stand-in (Node has no DOM).
function fakeDT(): DataTransfer {
  const store: Record<string, string> = {};
  return {
    setData: (k: string, v: string) => { store[k] = v; },
    getData: (k: string) => store[k] ?? '',
    get types() { return Object.keys(store); },
    effectAllowed: 'none',
  } as unknown as DataTransfer;
}

const dt = fakeDT();
writeDragPayload(dt, { sessionId: 's1', sourceTileId: 'A' });
assert(isTileTabDrag(dt), 'recognises tile-tab drag');
assert(dt.getData(TILE_TAB_MIME).length > 0, 'writes to the MIME slot');

const p = readDragPayload(dt);
assert(p !== null && p.sessionId === 's1' && p.sourceTileId === 'A', 'round-trips payload');

// A drag carrying only OS files is not a tile-tab drag
const files = fakeDT();
(files as any).setData('Files', 'x');
assert(!isTileTabDrag(files), 'ignores OS file drag');
assert(readDragPayload(files) === null, 'no payload from foreign drag');
assert(isTileTabDrag(null) === false, 'null-safe');

console.log('OK dragPayload');
