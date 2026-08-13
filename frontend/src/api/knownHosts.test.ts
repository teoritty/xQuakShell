import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import { removeKnownHost } from './knownHosts';
import { lastError } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

async function run() {
  let fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);

  // There is deliberately no addKnownHost wrapper. Writing trust for a host is only reachable
  // through resolveHostKeyRpc, which acts on a key the backend saw in a real handshake. A binding
  // that took a host and a key from the frontend let anything holding window.go pre-seed trust for
  // a host the user had never connected to, and the TOFU prompt then never fired.
  fake.program('RemoveKnownHost', undefined);
  await removeKnownHost('host1');
  let call = fake.calls.find((c) => c.method === 'RemoveKnownHost');
  assert(!!call && call.args[0] === 'host1', 'RemoveKnownHost called with host');

  assert(get(lastError) === null, 'no error reported for successful calls');

  // fallback behavior on failure
  fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);

  fake.program('RemoveKnownHost', () => { throw new Error('boom'); });
  lastError.set(null);
  await removeKnownHost('host1'); // should not throw
  assert(get(lastError) !== null, 'removeKnownHost failure reports error');

  // no gateway
  setGateway(null as any);
  lastError.set(null);
  await removeKnownHost('host1'); // should not throw
  assert(get(lastError) === null, 'no error reported when gateway is missing');

  console.log('knownHosts.test.ts: all assertions passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
