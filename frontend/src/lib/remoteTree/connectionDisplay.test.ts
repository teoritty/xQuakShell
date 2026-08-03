import { hasPingResult, pingColor } from './connectionDisplay';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

type Ping = { reachable?: boolean; latencyMs?: number };

// hasPingResult: true only when an entry exists (a ping has completed).
{
  const map = new Map<string, Ping>([['a', { reachable: true, latencyMs: 20 }]]);
  assert(hasPingResult(map, 'a') === true, 'existing entry → true');
  assert(hasPingResult(map, 'missing') === false, 'no entry → false (ping pending)');
}

// pingColor: unreachable host renders grey, not red.
{
  const map = new Map<string, Ping>([['down', { reachable: false, latencyMs: 0 }]]);
  const c = pingColor(map, 'down');
  assert(c.includes('text-secondary') || c.includes('9e9e9e'), `unreachable → grey, got ${c}`);
  assert(!c.includes('danger') && !c.includes('f44747'), `unreachable must not be red, got ${c}`);
}

// pingColor: reachable latencies keep their graded colors.
{
  const map = new Map<string, Ping>([
    ['fast', { reachable: true, latencyMs: 20 }],
    ['mid', { reachable: true, latencyMs: 150 }],
    ['slow', { reachable: true, latencyMs: 500 }],
  ]);
  assert(pingColor(map, 'fast') === '#4caf50', 'fast → green');
  assert(pingColor(map, 'mid') === '#ffb300', 'mid → amber');
  assert(pingColor(map, 'slow') === '#ff6f00', 'slow → orange');
}

// pingColor: no entry → transparent (the spinner is shown instead by the view).
assert(pingColor(new Map(), 'x') === 'transparent', 'no entry → transparent');

console.log('connectionDisplay.test.ts: all passed');
