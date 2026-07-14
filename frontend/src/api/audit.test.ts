import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  searchAuditLog,
  deleteAuditEntry,
  clearAuditLog,
  getAuditSessionState,
  enableAuditSecretLogging,
  disableAuditSecretLogging,
} from './audit';
import { lastError } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

function withoutMethod<T extends object>(target: T, methodName: string): T {
  return new Proxy(target, {
    get(t, prop: string) {
      if (prop === methodName) return undefined;
      return (t as unknown as Record<string, unknown>)[prop];
    },
  }) as T;
}

async function run() {
  let fake = createFakeGateway();
  setGateway(fake);
  lastError.set(null);

  // searchAuditLog returns [] when SearchAuditLog is absent on the gateway
  let noMethod = withoutMethod(fake, 'SearchAuditLog');
  setGateway(noMethod);
  let results = await searchAuditLog('q', 's', 'c');
  assert(Array.isArray(results) && results.length === 0, 'searchAuditLog returns [] when method absent');

  // searchAuditLog: representative pass-through + fallback to []
  fake = createFakeGateway();
  const entry = { id: 1, timestamp: 't', category: 'cmd', sessionId: 's1', connectionId: 'c1', connectionName: 'C', host: 'h', username: 'u', input: 'ls', redacted: false };
  fake.program('SearchAuditLog', [entry]);
  setGateway(fake);
  results = await searchAuditLog('q', 's1', 'c1', 'cmd', 100, 0);
  assert(results.length === 1 && results[0].id === 1, 'searchAuditLog returns entries from gateway');
  let call = fake.calls.find((c) => c.method === 'SearchAuditLog');
  assert(!!call && call.args[0] === 'q' && call.args[4] === 100, 'searchAuditLog called with args');

  fake.program('SearchAuditLog', () => { throw new Error('boom'); });
  lastError.set(null);
  results = await searchAuditLog('q', '', '');
  assert(Array.isArray(results) && results.length === 0, 'searchAuditLog falls back to [] on error');
  assert(get(lastError) !== null, 'searchAuditLog reports error via lastError');

  // deleteAuditEntry: no-ops when method absent
  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'DeleteAuditEntry');
  setGateway(noMethod);
  lastError.set(null);
  await deleteAuditEntry(1);
  assert(fake.calls.length === 0, 'deleteAuditEntry does not call gateway when method absent');

  fake = createFakeGateway();
  fake.program('DeleteAuditEntry', undefined);
  setGateway(fake);
  await deleteAuditEntry(5);
  call = fake.calls.find((c) => c.method === 'DeleteAuditEntry');
  assert(!!call && call.args[0] === 5, 'deleteAuditEntry called with id');

  // clearAuditLog
  fake = createFakeGateway();
  fake.program('ClearAuditLog', undefined);
  setGateway(fake);
  await clearAuditLog('cmd');
  call = fake.calls.find((c) => c.method === 'ClearAuditLog');
  assert(!!call && call.args[0] === 'cmd', 'clearAuditLog called with category');

  // getAuditSessionState: returns null when method absent, and silently on error
  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'GetAuditSessionState');
  setGateway(noMethod);
  let state = await getAuditSessionState();
  assert(state === null, 'getAuditSessionState returns null when method absent');

  fake = createFakeGateway();
  fake.program('GetAuditSessionState', () => { throw new Error('boom'); });
  setGateway(fake);
  lastError.set(null);
  state = await getAuditSessionState();
  assert(state === null, 'getAuditSessionState falls back to null on error');
  assert(get(lastError) === null, 'getAuditSessionState does not report error (silent catch)');

  fake = createFakeGateway();
  fake.program('GetAuditSessionState', { logSecretsEnabled: true });
  setGateway(fake);
  state = await getAuditSessionState();
  assert(state?.logSecretsEnabled === true, 'getAuditSessionState returns gateway result');

  // enableAuditSecretLogging: returns false when method absent, true on success
  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'EnableAuditSecretLogging');
  setGateway(noMethod);
  let ok = await enableAuditSecretLogging(true);
  assert(ok === false, 'enableAuditSecretLogging returns false when method absent');

  fake = createFakeGateway();
  fake.program('EnableAuditSecretLogging', undefined);
  setGateway(fake);
  ok = await enableAuditSecretLogging(true);
  assert(ok === true, 'enableAuditSecretLogging returns true on success');
  call = fake.calls.find((c) => c.method === 'EnableAuditSecretLogging');
  assert(!!call && call.args[0] === true, 'enableAuditSecretLogging called with confirmed flag');

  fake.program('EnableAuditSecretLogging', () => { throw new Error('boom'); });
  lastError.set(null);
  ok = await enableAuditSecretLogging(true);
  assert(ok === false, 'enableAuditSecretLogging returns false on error');
  assert(get(lastError) !== null, 'enableAuditSecretLogging reports error via lastError');

  // disableAuditSecretLogging: sync, no-op when method absent
  fake = createFakeGateway();
  noMethod = withoutMethod(fake, 'DisableAuditSecretLogging');
  setGateway(noMethod);
  disableAuditSecretLogging();
  assert(fake.calls.length === 0, 'disableAuditSecretLogging does not call gateway when method absent');

  fake = createFakeGateway();
  fake.program('DisableAuditSecretLogging', undefined);
  setGateway(fake);
  disableAuditSecretLogging();
  call = fake.calls.find((c) => c.method === 'DisableAuditSecretLogging');
  assert(!!call, 'disableAuditSecretLogging called');

  console.log('audit.test.ts passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
