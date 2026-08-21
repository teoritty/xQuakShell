import { appendCapped, capPending } from './buffer';

let failures = 0;
function assert(condition: boolean, message: string): void {
  if (!condition) {
    failures += 1;
    console.error('Error: ' + message);
  }
}

// An empty batch must hand back the very same array. Svelte invalidates on assignment, so a copy
// here would re-render every row on any frame that carried no new lines - the repaint storm this
// module exists to remove, reintroduced at a different rate.
{
  const existing = [{ seq: 1 }, { seq: 2 }];
  assert(appendCapped(existing, [], 10) === existing, 'an empty batch must return the same array reference');
}

// Order is the whole contract of a log: newest last, nothing reordered.
{
  const got = appendCapped([{ seq: 1 }, { seq: 2 }], [{ seq: 3 }, { seq: 4 }], 10);
  assert(got.map((r) => r.seq).join(',') === '1,2,3,4', 'appended lines must keep arrival order, got ' + got.map((r) => r.seq).join(','));
}

// The cap keeps the newest, because the newest is what a log window is read for.
{
  const got = appendCapped([{ seq: 1 }, { seq: 2 }, { seq: 3 }], [{ seq: 4 }], 2);
  assert(got.length === 2, 'the ring must hold exactly max lines, got ' + got.length);
  assert(got.map((r) => r.seq).join(',') === '3,4', 'the ring must keep the newest lines, got ' + got.map((r) => r.seq).join(','));
}

// A batch larger than the ring is itself trimmed rather than overflowing it.
{
  const got = appendCapped([{ seq: 1 }], [{ seq: 2 }, { seq: 3 }, { seq: 4 }], 2);
  assert(got.map((r) => r.seq).join(',') === '3,4', 'an oversized batch must be trimmed to the cap, got ' + got.map((r) => r.seq).join(','));
}

// Exactly at the cap nothing is dropped: the boundary, not one either side of it.
{
  const got = appendCapped([{ seq: 1 }], [{ seq: 2 }], 2);
  assert(got.map((r) => r.seq).join(',') === '1,2', 'a batch landing exactly on the cap must drop nothing, got ' + got.map((r) => r.seq).join(','));
}

// capPending bounds the batch that accumulates while requestAnimationFrame is not firing - a
// minimised window is the case, and without this the pending array grows for as long as it stays
// minimised.
{
  const got = capPending([{ seq: 1 }, { seq: 2 }, { seq: 3 }], 2);
  assert(got.map((r) => r.seq).join(',') === '2,3', 'capPending must keep the newest, got ' + got.map((r) => r.seq).join(','));

  const under = [{ seq: 1 }];
  assert(capPending(under, 5) === under, 'capPending must not copy a batch that is under the cap');
}

if (failures > 0) {
  process.exit(1);
}
console.log('logviewer/buffer.test.ts: all passed');
