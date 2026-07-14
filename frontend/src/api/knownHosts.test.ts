import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import { addKnownHost, removeKnownHost } from './knownHosts';
import { lastError } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

async function run() {
  let fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);

  fake.program('AddKnownHost', undefined);
  await addKnownHost('host1', 'keyBase64');
  let call = fake.calls.find((c) => c.method === 'AddKnownHost');
  assert(!!call && call.args[0] === 'host1' && call.args[1] === 'keyBase64', 'AddKnownHost called with args');

  fake.program('RemoveKnownHost', undefined);
  await removeKnownHost('host1');
  call = fake.calls.find((c) => c.method === 'RemoveKnownHost');
  assert(!!call && call.args[0] === 'host1', 'RemoveKnownHost called with host');

  assert(get(lastError) === null, 'no error reported for successful calls');

  // fallback behavior on failure
  fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);

  fake.program('AddKnownHost', () => { throw new Error('boom'); });
  await addKnownHost('host1', 'keyBase64'); // should not throw
  assert(get(lastError) !== null, 'addKnownHost failure reports error');

  fake.program('RemoveKnownHost', () => { throw new Error('boom'); });
  lastError.set(null);
  await removeKnownHost('host1'); // should not throw
  assert(get(lastError) !== null, 'removeKnownHost failure reports error');

  // no gateway
  setGateway(null as any);
  lastError.set(null);
  await addKnownHost('host1', 'keyBase64'); // should not throw
  await removeKnownHost('host1'); // should not throw
  assert(get(lastError) === null, 'no error reported when gateway is missing');

  console.log('knownHosts.test.ts: all assertions passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
