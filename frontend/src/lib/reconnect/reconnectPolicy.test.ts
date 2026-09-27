import assert from 'node:assert/strict';
import {
  RECONNECT_DELAYS_SECONDS,
  reconnectDecision,
  reconnectDelaySeconds,
  secondsRemaining,
} from './reconnectPolicy';

// The schedule the user asked for: 5, 15, 30, then 60 for every attempt after that.
{
  const walked = [1, 2, 3, 4, 5, 6, 50].map(reconnectDelaySeconds);
  assert.deepEqual(walked, [5, 15, 30, 60, 60, 60, 60], 'the delay grows 5-15-30-60 and then stays at 60');
  assert.deepEqual([...RECONNECT_DELAYS_SECONDS], [5, 15, 30, 60], 'the published schedule is the one the UI describes');
}

// A malformed attempt number must still produce a real delay, never undefined - an undefined
// delay becomes setTimeout(fn, NaN), which fires at once and turns the backoff into a busy loop.
{
  assert.equal(reconnectDelaySeconds(0), 5, 'attempt 0 is treated as the first attempt');
  assert.equal(reconnectDelaySeconds(-3), 5, 'a negative attempt is treated as the first attempt');
  assert.equal(reconnectDelaySeconds(2.7), 15, 'a fractional attempt rounds down to a real one');
}

// The digit rounds up and bottoms out at zero.
{
  assert.equal(secondsRemaining(10_000, 5_000), 5, 'exactly five seconds left reads 5');
  assert.equal(secondsRemaining(10_000, 5_001), 5, 'a sliver under five seconds still reads 5');
  assert.equal(secondsRemaining(10_000, 9_999), 1, 'one millisecond left reads 1, not 0');
  assert.equal(secondsRemaining(10_000, 10_000), 0, 'the deadline itself reads 0');
  assert.equal(secondsRemaining(10_000, 12_000), 0, 'a passed deadline never goes negative');
}

// The decision table. Each row names the rule it pins.
{
  const rows: [Parameters<typeof reconnectDecision>, string, string][] = [
    [['ready', { state: 'error', connectionLost: true }, false], 'schedule', 'an established session that lost its link is retried'],
    [['ready', { state: 'error', connectionLost: false }, false], 'none', 'a shell that exited on its own is not reopened'],
    [['connecting', { state: 'error' }, false], 'none', 'a failed first connect is not retried on a timer'],
    [['connecting', { state: 'error' }, true], 'schedule', 'a failed automatic attempt schedules the next one'],
    [[undefined, { state: 'error', connectionLost: true }, false], 'schedule', 'a loss seen on first sight still counts'],
    [['error', { state: 'error', connectionLost: true }, false], 'none', 'republishing an error must not stack a second timer'],
    [['error', { state: 'error' }, true], 'none', 'nor for an attempt that is already failed'],
    [['connecting', { state: 'ready' }, true], 'reset', 'reaching ready ends the chain'],
    [['ready', { state: 'ready' }, false], 'reset', 'ready is always a reset, even without a chain'],
    [['error', { state: 'connecting' }, true], 'none', 'connecting is neither a failure nor a success'],
    [['connecting', { state: 'hostkey-required' }, true], 'none', 'a question for the user pauses the chain rather than retrying it'],
    [['connecting', { state: 'trust-required' }, true], 'none', 'so does a peer-trust question'],
  ];
  for (const [args, want, rule] of rows) {
    assert.equal(reconnectDecision(...args), want, rule);
  }
}

console.log('reconnectPolicy.test passed');
