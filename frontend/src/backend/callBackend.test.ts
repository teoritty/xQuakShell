import { setGateway } from './context';
import { createFakeGateway } from './fakeGateway';
import { callBackend, callBackendVoid } from './callBackend';
import { lastError } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

async function run() {
  // success
  setGateway(createFakeGateway());
  lastError.set(null);
  let r = await callBackend('Ctx', 'fb', async () => 'ok');
  assert(r === 'ok', 'returns value on success');
  assert(get(lastError) === null, 'no error on success');

  // failure -> fallback + error
  r = await callBackend('Load X', 'fb', async () => { throw new Error('boom'); });
  assert(r === 'fb', 'returns fallback on failure');
  assert(get(lastError)?.message === 'Load X: boom', 'reports contextual error');

  // silence predicate
  lastError.set(null);
  await callBackend('Get settings', null, async () => { throw new Error('vault is locked'); },
    { silence: (msg) => msg.includes('vault is locked') });
  assert(get(lastError) === null, 'silenced error is not reported');

  // rethrow after reporting
  lastError.set(null);
  let threw = false;
  try {
    await callBackend('Save plugin settings', undefined, async () => { throw new Error('disk full'); },
      { rethrow: true });
  } catch (e) {
    threw = true;
    assert(e instanceof Error && e.message === 'disk full', 'rethrown error preserved');
  }
  assert(threw, 'rethrow:true re-throws after reporting');
  assert(get(lastError)?.message === 'Save plugin settings: disk full', 'reports before rethrow');

  // missing gateway
  setGateway(null);
  let called = false;
  r = await callBackend('Ctx', 'fb', async () => { called = true; return 'x'; });
  assert(!called && r === 'fb', 'skips fn and returns fallback when no gateway');

  // callBackendVoid
  setGateway(createFakeGateway());
  lastError.set(null);
  let voidCalled = false;
  await callBackendVoid('Void ctx', async () => { voidCalled = true; });
  assert(voidCalled, 'callBackendVoid invokes fn');
  assert(get(lastError) === null, 'callBackendVoid no error on success');

  console.log('callBackend.test passed');
}

run();
