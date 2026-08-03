import { pingColor, pingStatus, pingTooltip } from './connectionDisplay';
import { isValidStatusColor, statusDotColor, statusDotTooltip } from './statusDot';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

// --- tone mapping ---
assert(statusDotColor({ tone: 'ok' }) === '#4caf50', 'ok → green');
assert(statusDotColor({ tone: 'warn' }) === '#ffb300', 'warn → amber');
assert(statusDotColor({ tone: 'error' }) === 'var(--danger, #f44747)', 'error → theme danger');
assert(statusDotColor({ tone: 'neutral' }) === 'var(--text-secondary, #9e9e9e)', 'neutral → theme grey');
assert(statusDotColor({ tone: 'busy' }) === 'var(--accent, #4b8bbf)', 'busy → theme accent');
assert(statusDotColor({ tone: 'unknown' }) === 'var(--text-disabled, #6e6e6e)', 'unknown → theme disabled');
assert(
  statusDotColor({ tone: 'neutral' }) !== statusDotColor({ tone: 'unknown' }),
  '"the plugin says neutral" and "the plugin does not know" are different states'
);

// --- null status draws nothing; the caller decides what fills the slot ---
assert(statusDotColor(null) === 'transparent', 'null → transparent');
assert(statusDotColor(undefined) === 'transparent', 'undefined → transparent');
assert(statusDotTooltip(null) === '', 'null → no tooltip');

// --- a valid override wins ---
assert(statusDotColor({ tone: 'ok', color: '#123abc' }) === '#123abc', 'valid hex overrides the tone');
assert(statusDotColor({ tone: 'ok', color: '#ABCDEF' }) === '#ABCDEF', 'uppercase hex is valid');

// --- anything else is ignored, NOT sanitized and NOT fatal ---
for (const bad of [
  '#abc',
  '#12345',
  '#1234567',
  'red',
  'rgb(1,2,3)',
  'var(--danger)',
  '#12345g',
  ' #123456',
  '#123456;background:url(x)',
  'javascript:alert(1)',
  '',
]) {
  assert(!isValidStatusColor(bad), `"${bad}" must not validate`);
  assert(
    statusDotColor({ tone: 'ok', color: bad }) === '#4caf50',
    `invalid colour "${bad}" falls back to the tone rather than reaching CSS`
  );
}
assert(!isValidStatusColor(undefined), 'a missing colour is not a valid colour');

// --- tooltip passthrough ---
assert(statusDotTooltip({ tone: 'ok', tooltip: 'Running' }) === 'Running', 'tooltip is passed through');
assert(statusDotTooltip({ tone: 'ok' }) === '', 'no tooltip → empty string, not "undefined"');

// --- the ping now goes through the same primitive and renders identically ---
{
  const map = new Map<string, { reachable?: boolean; latencyMs?: number }>([
    ['fast', { reachable: true, latencyMs: 20 }],
    ['mid', { reachable: true, latencyMs: 150 }],
    ['slow', { reachable: true, latencyMs: 500 }],
    ['dead', { reachable: true, latencyMs: 4000 }],
    ['down', { reachable: false }],
  ]);
  for (const id of ['fast', 'mid', 'slow', 'dead', 'down', 'absent']) {
    assert(
      statusDotColor(pingStatus(map, id)) === pingColor(map, id),
      `ping colour for ${id} must be the StatusDot colour, not a second implementation`
    );
  }
  assert(pingStatus(map, 'absent') === null, 'no ping result → null, so the view draws a spinner');
  assert(pingTooltip(map, 'absent') === 'Not pinged yet', 'the spinner case keeps its own tooltip');
  assert(pingStatus(map, 'slow')?.color === '#ff6f00', 'the 300-999ms band uses a valid hex override');
  assert(isValidStatusColor(pingStatus(map, 'slow')?.color), 'that override passes the same validation a plugin gets');
}

console.log('statusDot.test.ts: all passed');
