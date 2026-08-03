import assert from 'node:assert';
import {
  appendPendingTerminalOutput,
  clearPendingTerminalOutput,
  clearTerminalOutputConsumer,
  hasTerminalOutputConsumer,
  registerTerminalOutputConsumer,
  takePendingTerminalOutput,
} from './outputBuffer';

// Byte-limit truncation: appending past MAX_PENDING_TERMINAL_BYTES (256 << 10 = 262144)
// drops oldest chunks first, but always keeps at least one chunk (trimmed from the front
// if that single remaining chunk is itself over the limit).
{
  const sessionId = 'trunc-1';
  const MAX = 256 << 10;
  // Two chunks that together exceed the limit: dropping the oldest should suffice.
  const chunkA = new Uint8Array(MAX - 100).fill(1);
  const chunkB = new Uint8Array(200).fill(2);
  appendPendingTerminalOutput(sessionId, chunkA);
  appendPendingTerminalOutput(sessionId, chunkB);
  const chunks = takePendingTerminalOutput(sessionId);
  assert.strictEqual(chunks.length, 1, 'oldest chunk should be dropped, keeping only the newest');
  assert.strictEqual(chunks[0], chunkB, 'the surviving chunk should be the newest one');
}

{
  // A single chunk larger than MAX must still be kept (trimmed from the front), never dropped entirely.
  const sessionId = 'trunc-2';
  const MAX = 256 << 10;
  const huge = new Uint8Array(MAX + 500).fill(3);
  appendPendingTerminalOutput(sessionId, huge);
  const chunks = takePendingTerminalOutput(sessionId);
  assert.strictEqual(chunks.length, 1, 'single oversized chunk is kept, not dropped');
  assert.strictEqual(chunks[0].length, MAX, 'oversized single chunk is trimmed down to the limit');
}

// take clears the buffer
{
  const sessionId = 'take-clears';
  appendPendingTerminalOutput(sessionId, new Uint8Array([1, 2, 3]));
  const first = takePendingTerminalOutput(sessionId);
  assert.strictEqual(first.length, 1);
  const second = takePendingTerminalOutput(sessionId);
  assert.strictEqual(second.length, 0, 'buffer must be empty after take');
}

// clearPendingTerminalOutput empties the buffer without returning anything
{
  const sessionId = 'clear-explicit';
  appendPendingTerminalOutput(sessionId, new Uint8Array([9]));
  clearPendingTerminalOutput(sessionId);
  const chunks = takePendingTerminalOutput(sessionId);
  assert.strictEqual(chunks.length, 0);
}

// Consumer refcount: overlapping register/release calls (the tile-rearrange scenario) must
// keep the session marked as "has consumer" until every register has a matching release.
{
  const sessionId = 'refcount-1';
  assert.strictEqual(hasTerminalOutputConsumer(sessionId), false);

  const releaseOld = registerTerminalOutputConsumer(sessionId);
  assert.strictEqual(hasTerminalOutputConsumer(sessionId), true);

  // New component mounts and registers before the old one unmounts (overlap).
  const releaseNew = registerTerminalOutputConsumer(sessionId);
  assert.strictEqual(hasTerminalOutputConsumer(sessionId), true);

  // Old component unmounts and releases first: still consumed because the new one is live.
  releaseOld();
  assert.strictEqual(hasTerminalOutputConsumer(sessionId), true, 'must stay consumed while an overlapping registration is active');

  // New component eventually releases too: now unconsumed.
  releaseNew();
  assert.strictEqual(hasTerminalOutputConsumer(sessionId), false);
}

// Calling the same release closure twice is a no-op (doesn't double-decrement).
{
  const sessionId = 'refcount-double-release';
  const releaseA = registerTerminalOutputConsumer(sessionId);
  const releaseB = registerTerminalOutputConsumer(sessionId);
  releaseA();
  releaseA(); // no-op, already released
  assert.strictEqual(hasTerminalOutputConsumer(sessionId), true, 'second registration still active');
  releaseB();
  assert.strictEqual(hasTerminalOutputConsumer(sessionId), false);
}

// clearTerminalOutputConsumer forcibly removes the entry (used on SessionClosed).
{
  const sessionId = 'force-clear';
  registerTerminalOutputConsumer(sessionId);
  assert.strictEqual(hasTerminalOutputConsumer(sessionId), true);
  clearTerminalOutputConsumer(sessionId);
  assert.strictEqual(hasTerminalOutputConsumer(sessionId), false);
}

console.log('outputBuffer.test.ts: all assertions passed');
