import { getGateway, setGateway, getRuntime, setRuntime } from './context';
import { createFakeGateway } from './fakeGateway';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error(msg);
}

// Injection round-trips for the app gateway.
const fake = createFakeGateway();
setGateway(fake);
assert(getGateway() === fake, 'injected gateway should be returned');
setGateway(null);
assert(getGateway() === null, 'gateway resets to null');

// Injection round-trips for the runtime gateway.
const fakeRuntime = { EventsOn: () => {} };
setRuntime(fakeRuntime);
assert(getRuntime() === fakeRuntime, 'injected runtime should be returned');
setRuntime(null);
assert(getRuntime() === null, 'runtime resets to null');

// FakeGateway records calls and returns programmed results.
const fake2 = createFakeGateway();
fake2.program('GetPlatform', 'linux');
fake2.GetPlatform().then((result) => {
  assert(result === 'linux', 'programmed result should be returned');
  assert(fake2.calls.length === 1, 'call should be recorded');
  assert(fake2.calls[0].method === 'GetPlatform', 'recorded method name should match');

  // program() itself must not be recorded as a call.
  assert(
    fake2.calls.every((c) => c.method !== 'program'),
    'program() must not be recorded as a call'
  );

  // A programmed function that throws should surface as a rejected promise.
  const fake3 = createFakeGateway();
  fake3.program('DeleteFolder', () => {
    throw new Error('boom');
  });
  fake3
    .DeleteFolder('x')
    .then(() => {
      throw new Error('expected rejection');
    })
    .catch((e: Error) => {
      assert(e.message === 'boom', 'thrown error should propagate via rejection');
      console.log('context.test passed');
    });
});
