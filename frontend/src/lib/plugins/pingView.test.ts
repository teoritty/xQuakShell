// What the Ping button reports, asserted without mounting a component.
//
// The button had no result handling at all: a plugin that answered produced nothing on screen, and
// a plugin that did not answer produced the application's error modal - the same dialog a crash
// uses. Both halves are pinned here, because "a failed ping is a finding about the plugin, not a
// failure of the app" is a decision that is easy to undo by accident.
import {
  formatPong,
  pingFailed,
  pingStatus,
  pingSucceeded,
  PING_RESULT_TTL_MS,
} from './pingView';

function assert(cond: boolean, msg: string): void {
  if (!cond) throw new Error('FAIL: ' + msg);
}

// --- the payload the plugin answered with ---

assert(
  formatPong({ version: '1.0.5', pong: 'ok' }) === 'pong=ok version=1.0.5',
  `formatPong = ${formatPong({ version: '1.0.5', pong: 'ok' })}; a payload renders as key=value pairs`
);

// Sorting is not cosmetic. The payload arrives as a decoded JSON object, and showing the same
// answer in two different orders on two consecutive pings reads as the plugin having changed its
// reply.
assert(
  formatPong({ pong: 'ok', version: '1.0.5' }) === formatPong({ version: '1.0.5', pong: 'ok' }),
  'the rendering must not depend on the order the keys arrived in'
);

assert(formatPong({}) === '', 'an empty payload renders as nothing, not as "{}"');

// --- a ping that answered ---

const fast = pingSucceeded(4.2, { pong: 'ok' });
assert(fast.ok, 'an answered ping is ok');
assert(fast.ms === 4, `ms = ${fast.ms}; the round trip rounds to the nearest millisecond`);
assert(fast.detail === 'pong=ok', `detail = ${fast.detail}; the payload is what the tooltip shows`);

// A plugin on a local pipe answers in well under a millisecond. Reporting that as "0 ms" reads as a
// measurement that failed, which is the opposite of what happened.
assert(pingSucceeded(0.3, {}).ms === 1, 'a sub-millisecond answer is reported as 1 ms, never 0');
assert(pingSucceeded(7.6, {}).ms === 8, 'rounding, not truncation: 7.6 ms is 8 ms');

// performance.now() differences are not always usable - a suspended machine, a mocked clock - and a
// NaN reaching the status line renders as "Answered in NaN ms".
assert(pingSucceeded(Number.NaN, {}).ms === 0, 'an unusable measurement reports 0 rather than NaN');
assert(pingSucceeded(-5, {}).ms === 0, 'a negative interval reports 0');

// --- a ping that did not answer ---

const failed = pingFailed(120, new Error('plugin ping: plugin is not running'));
assert(!failed.ok, 'an unanswered ping is not ok');
assert(failed.ms === 120, `ms = ${failed.ms}; how long the user waited is part of the answer`);
assert(
  failed.detail === 'plugin ping: plugin is not running',
  `detail = ${failed.detail}; the backend's own reason is carried through verbatim`
);

// Wails rejects with whatever the Go side produced, and that is not always an Error. A thrown
// string or object used to reach the user as "[object Object]" through the global dialog.
assert(
  pingFailed(5, 'context deadline exceeded').detail === 'context deadline exceeded',
  'a thrown string is a reason too'
);
assert(pingFailed(5, { code: 7 }).detail !== '', 'a thrown object still produces some reason');

// --- how the row shows it ---

const okStatus = pingStatus(fast);
assert(okStatus.key === 'plugins.ping.ok', `key = ${okStatus.key}`);
assert(okStatus.values.ms === 4, 'the latency is handed to the message for interpolation');
assert(okStatus.kind === 'installed', 'an answer is shown in the same tone as "Running"');

// Warning, not the danger tone the global error dialog uses. A plugin that does not answer is a
// finding about that plugin; colouring it as a crash is how this button came to feel broken.
const failStatus = pingStatus(failed);
assert(failStatus.key === 'plugins.ping.failed', `key = ${failStatus.key}`);
assert(
  failStatus.kind === 'warning',
  `kind = ${failStatus.kind}; a silent plugin is a warning on its row, not an application error`
);

// The result is transient by design: it borrows the status line and must give it back, or the row
// stops reporting run state for the rest of the session.
assert(
  PING_RESULT_TTL_MS > 0 && PING_RESULT_TTL_MS <= 30000,
  `TTL = ${PING_RESULT_TTL_MS}ms; long enough to read, short enough to give the line back`
);

console.log('pingView.test passed');
