import { hasPingResult, pingColor, protocolBadge } from './connectionDisplay';
import type { ConnectionProtocol } from '../../api/protocolTypes';

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

// protocolBadge -----------------------------------------------------------

const ssh: ConnectionProtocol = { id: 'ssh', label: 'SSH', defaultPort: 22, icon: 'terminal', remoteFs: true };
const rdp: ConnectionProtocol = { id: 'rdp', label: 'Remote Desktop', defaultPort: 3389, icon: 'monitor', remoteFs: false };

// A stock install has one protocol and every row is SSH, so a column of identical tags would say
// nothing. The badge exists to tell rows apart, and there is nothing to tell apart yet.
{
  assert(protocolBadge('ssh', [ssh]) === null, 'one registered protocol → no badge');
  assert(protocolBadge(undefined, [ssh]) === null, 'one registered protocol → no badge for a bare connection either');
  assert(protocolBadge('rdp', []) === null, 'an empty protocol list cannot produce a badge');
}

// The moment a plugin registers a second protocol, every row is tagged - including the SSH ones,
// which is what stops "no badge" from having to be read as a third, invisible state.
{
  const badge = protocolBadge('rdp', [ssh, rdp]);
  assert(badge !== null, 'two registered protocols → badges appear');
  assert(badge?.text === 'RDP', `badge text = ${badge?.text}, want RDP`);

  const sshBadge = protocolBadge('ssh', [ssh, rdp]);
  assert(sshBadge?.text === 'SSH', `ssh row is tagged too, got ${sshBadge?.text}`);
}

// A connection saved before plugins existed carries no protocol at all. The backend reads that as
// SSH, and a blank tag on those rows would be the one row shape nobody can interpret.
{
  assert(protocolBadge(undefined, [ssh, rdp])?.text === 'SSH', 'undefined protocol reads as SSH');
  assert(protocolBadge('', [ssh, rdp])?.text === 'SSH', 'empty protocol reads as SSH');
}

// The id is what is shown, not the plugin's label: the badge takes its width out of the connection
// name, so a plugin calling itself "Remote Desktop" must not be able to eat half of every row. The
// label is still reachable, in the tooltip.
{
  const badge = protocolBadge('rdp', [ssh, rdp]);
  assert(badge?.text === 'RDP', 'the id is shown, capitalised');
  assert(badge?.title === 'Remote Desktop', `the label is the tooltip, got ${badge?.title}`);
  assert(badge!.text.length <= 8, 'the shown text stays short enough to sit at a fixed edge');
}

// A protocol id with no matching entry still gets a tag. This is a connection whose plugin has been
// uninstalled: showing the row untagged would claim it is SSH, which is exactly wrong.
{
  const badge = protocolBadge('telnet', [ssh, rdp]);
  assert(badge?.text === 'TELNET', `unknown protocol still tagged, got ${badge?.text}`);
  assert(badge?.title === 'TELNET', 'with no label to show, the tooltip falls back to the tag');
}

console.log('connectionDisplay.test.ts: all passed');
