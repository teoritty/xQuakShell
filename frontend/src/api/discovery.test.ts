import { get } from 'svelte/store';
import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import { lastError } from '../stores/appState';
import { getDiscoveryTree, invokeDiscoveryAction, setDiscoveryObserved } from './discovery';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

async function run() {
  let fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);

  // --- happy path: arguments are forwarded verbatim ---
  fake.program('GetDiscoveryTree', { connectionId: 'c1', plugins: [{ pluginId: 'p1', nodes: [], branches: {} }] });
  const snapshot = await getDiscoveryTree('c1');
  assert(snapshot.plugins.length === 1, 'snapshot is returned as-is');
  let call = fake.calls.find((c) => c.method === 'GetDiscoveryTree');
  assert(!!call && call.args[0] === 'c1', 'GetDiscoveryTree is addressed by connectionId');

  fake.program('SetDiscoveryObserved', undefined);
  await setDiscoveryObserved('c1', ['', 'containers']);
  call = fake.calls.find((c) => c.method === 'SetDiscoveryObserved');
  assert(!!call && call.args[0] === 'c1', 'observe is addressed by connectionId');
  assert(
    JSON.stringify(call!.args[1]) === JSON.stringify(['', 'containers']),
    'observe carries the FULL node id set, root included'
  );

  fake.program('InvokeDiscoveryAction', undefined);
  await invokeDiscoveryAction('c1', 'p1', ['n1', 'n2'], 'stop');
  call = fake.calls.find((c) => c.method === 'InvokeDiscoveryAction');
  assert(!!call, 'InvokeDiscoveryAction called');
  assert(call!.args[0] === 'c1' && call!.args[1] === 'p1', 'the action names its plugin, not just its nodes');
  assert(JSON.stringify(call!.args[2]) === JSON.stringify(['n1', 'n2']), 'nodeIds is always a list');
  assert(call!.args[3] === 'stop', 'actionId is passed through opaquely');
  assert(get(lastError) === null, 'no error reported on the happy path');

  // --- "no tree" is an ordinary state, never a null the caller must handle ---
  fake.program('GetDiscoveryTree', null);
  let empty = await getDiscoveryTree('c1');
  assert(Array.isArray(empty.plugins) && empty.plugins.length === 0, 'a null answer normalizes to an empty snapshot');
  assert(empty.connectionId === 'c1', 'and still names the connection it was asked about');
  fake.program('GetDiscoveryTree', { connectionId: 'c1' });
  empty = await getDiscoveryTree('c1');
  assert(Array.isArray(empty.plugins), 'a snapshot without a plugins array is normalized too');

  // --- failures report and fall back, they do not throw at the caller ---
  fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);
  fake.program('GetDiscoveryTree', () => {
    throw new Error('boom');
  });
  const failed = await getDiscoveryTree('c1');
  assert(failed.plugins.length === 0, 'a failed fetch degrades to an empty subtree');
  assert(get(lastError) !== null, 'and is reported');

  lastError.set(null);
  fake.program('SetDiscoveryObserved', () => {
    throw new Error('boom');
  });
  await setDiscoveryObserved('c1', ['']);
  assert(get(lastError) !== null, 'a failed observe is reported');

  lastError.set(null);
  fake.program('InvokeDiscoveryAction', () => {
    throw new Error('boom');
  });
  await invokeDiscoveryAction('c1', 'p1', ['n1'], 'stop');
  assert(get(lastError) !== null, 'a failed action is reported');

  // --- an older backend simply does not expose these methods ---
  setGateway({} as any);
  lastError.set(null);
  const none = await getDiscoveryTree('c1');
  assert(none.plugins.length === 0, 'missing method → empty snapshot');
  await setDiscoveryObserved('c1', ['']);
  await invokeDiscoveryAction('c1', 'p1', ['n1'], 'stop');
  assert(get(lastError) === null, 'a backend without discovery is not an error to show the user');

  setGateway(null as any);
  lastError.set(null);
  assert((await getDiscoveryTree('c1')).plugins.length === 0, 'no gateway → empty snapshot');
  await setDiscoveryObserved('c1', ['']);
  await invokeDiscoveryAction('c1', 'p1', ['n1'], 'stop');
  assert(get(lastError) === null, 'no gateway is not an error either');

  console.log('discovery.test.ts: all assertions passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
