import { createRateTracker } from './transferRate';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}
function approx(a: number, b: number, eps = 1e-6): boolean {
  return Math.abs(a - b) <= eps;
}

// First sample has no prior reference → rate 0.
{
  const t = createRateTracker();
  assert(t.sample('a', 0, 1000) === 0, 'first sample → 0');
}

// Steady stream: 1 MiB per second. EMA converges toward the true rate.
{
  const t = createRateTracker();
  const MiB = 1024 * 1024;
  t.sample('a', 0, 0);
  let r = 0;
  for (let i = 1; i <= 20; i++) r = t.sample('a', MiB * i, i * 1000);
  assert(approx(r, MiB, MiB * 0.02), `converges to ~1 MiB/s, got ${r}`);
}

// Δt <= 0 returns the prior smoothed rate unchanged (no divide-by-zero).
{
  const t = createRateTracker();
  t.sample('a', 0, 0);
  const r1 = t.sample('a', 1000, 1000); // 1000 B/s instantaneous, EMA from 0
  const r2 = t.sample('a', 2000, 1000); // same timestamp → unchanged
  assert(r1 === r2, 'Δt<=0 keeps prior rate');
}

// Terminal reset (done drops back to 0) must not read as negative speed.
{
  const t = createRateTracker();
  t.sample('a', 5000, 0);
  const prev = t.sample('a', 10000, 1000);
  const afterReset = t.sample('a', 0, 2000); // done reset to 0 → Δdone < 0
  assert(afterReset === prev, 'negative Δdone ignored');
}

// clear() drops per-id state; a later sample behaves like a fresh transfer.
{
  const t = createRateTracker();
  t.sample('a', 0, 0);
  t.sample('a', 1024, 1000);
  t.clear('a');
  assert(t.sample('a', 999999, 5000) === 0, 'after clear, first sample → 0');
}

// Independent ids do not interfere.
{
  const t = createRateTracker();
  t.sample('a', 0, 0);
  t.sample('b', 0, 0);
  const ra = t.sample('a', 1000, 1000);
  const rb = t.sample('b', 2000, 1000);
  assert(ra !== rb, 'ids tracked independently');
}

console.log('transferRate.test.ts: all passed');
