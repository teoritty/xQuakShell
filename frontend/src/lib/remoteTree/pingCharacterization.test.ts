// Characterization test for the connection ping indicator.
//
// Written BEFORE the StatusDot refactor (ADR-014 / task 6) and deliberately
// pinned to the exact strings the tree rendered at that moment. Its only job is
// to fail if moving the ping onto the shared StatusDot primitive changes a
// colour, a threshold or a tooltip. It must pass identically before and after
// the refactor — do not "update" an expectation here to make a change pass;
// that is the change being wrong.
import { hasPingResult, pingColor, pingTooltip } from './connectionDisplay';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

type Ping = { reachable?: boolean; latencyMs?: number };

const map = new Map<string, Ping>([
  ['unreachable', { reachable: false, latencyMs: 0 }],
  ['unreachable-with-latency', { reachable: false, latencyMs: 42 }],
  ['fast-0', { reachable: true, latencyMs: 0 }],
  ['fast-99', { reachable: true, latencyMs: 99 }],
  ['amber-100', { reachable: true, latencyMs: 100 }],
  ['amber-299', { reachable: true, latencyMs: 299 }],
  ['orange-300', { reachable: true, latencyMs: 300 }],
  ['orange-999', { reachable: true, latencyMs: 999 }],
  ['red-1000', { reachable: true, latencyMs: 1000 }],
  ['red-9999', { reachable: true, latencyMs: 9999 }],
  ['no-latency', { reachable: true }],
]);

// --- presence: a missing entry means "not pinged yet" (view draws a spinner) ---
assert(hasPingResult(map, 'fast-0') === true, 'existing entry → hasPingResult true');
assert(hasPingResult(map, 'absent') === false, 'missing entry → hasPingResult false');
assert(pingColor(map, 'absent') === 'transparent', 'missing entry → transparent');
assert(pingTooltip(map, 'absent') === 'Not pinged yet', 'missing entry → "Not pinged yet"');

// --- unreachable is grey, never red, whatever the latency field says ---
assert(
  pingColor(map, 'unreachable') === 'var(--text-secondary, #9e9e9e)',
  'unreachable → grey'
);
assert(
  pingColor(map, 'unreachable-with-latency') === 'var(--text-secondary, #9e9e9e)',
  'unreachable ignores latency'
);
assert(pingTooltip(map, 'unreachable') === 'Unreachable', 'unreachable → "Unreachable"');

// --- the 100 / 300 / 1000 ms thresholds, pinned on both sides of each edge ---
assert(pingColor(map, 'fast-0') === '#4caf50', '0ms → green');
assert(pingColor(map, 'fast-99') === '#4caf50', '99ms → green');
assert(pingColor(map, 'amber-100') === '#ffb300', '100ms → amber');
assert(pingColor(map, 'amber-299') === '#ffb300', '299ms → amber');
assert(pingColor(map, 'orange-300') === '#ff6f00', '300ms → orange');
assert(pingColor(map, 'orange-999') === '#ff6f00', '999ms → orange');
assert(pingColor(map, 'red-1000') === 'var(--danger, #f44747)', '1000ms → red');
assert(pingColor(map, 'red-9999') === 'var(--danger, #f44747)', '9999ms → red');

// --- a reachable host with no latency field is treated as 0ms, i.e. green ---
assert(pingColor(map, 'no-latency') === '#4caf50', 'reachable without latency → green');

// --- tooltips are the raw latency with a bare "ms" suffix ---
assert(pingTooltip(map, 'fast-99') === '99ms', 'reachable → "<n>ms"');
assert(pingTooltip(map, 'red-1000') === '1000ms', 'slow reachable → "<n>ms"');
assert(pingTooltip(map, 'no-latency') === 'undefinedms', 'missing latency keeps its literal rendering');

console.log('pingCharacterization.test.ts: all passed');
