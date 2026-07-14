import { formatBytesPerSec } from './formatBytes';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

// Non-finite / non-positive → empty (caller hides the element)
assert(formatBytesPerSec(0) === '', 'zero → empty');
assert(formatBytesPerSec(-5) === '', 'negative → empty');
assert(formatBytesPerSec(NaN) === '', 'NaN → empty');
assert(formatBytesPerSec(Infinity) === '', 'Infinity → empty');

// Bytes range: no decimals
assert(formatBytesPerSec(1) === '1 B/s', '1 B/s');
assert(formatBytesPerSec(1023) === '1023 B/s', '1023 B/s');

// KiB boundary and rounding
assert(formatBytesPerSec(1024) === '1.0 KiB/s', '1024 → 1.0 KiB/s');
assert(formatBytesPerSec(1536) === '1.5 KiB/s', '1536 → 1.5 KiB/s');
assert(formatBytesPerSec(1024 * 812) === '812.0 KiB/s', '812 KiB/s');

// MiB
assert(formatBytesPerSec(1024 * 1024) === '1.0 MiB/s', '1 MiB/s');
assert(formatBytesPerSec(Math.round(1024 * 1024 * 3.4)) === '3.4 MiB/s', '3.4 MiB/s');

// GiB (top unit, does not overflow to TiB)
assert(formatBytesPerSec(1024 * 1024 * 1024) === '1.0 GiB/s', '1 GiB/s');
assert(formatBytesPerSec(1024 * 1024 * 1024 * 2048) === '2048.0 GiB/s', 'stays in GiB');

console.log('formatBytes.test.ts: all passed');
