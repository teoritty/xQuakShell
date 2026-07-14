import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  openSessionRpc,
  closeSessionRpc,
  reportEmbedViewport,
  reportEmbedActivity,
  getPlatform,
  resolveHostKeyRpc,
} from './sessions';
import { lastError, pendingHostKey } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

async function run() {
  // openSessionRpc: returns the id string forwarded by OpenSession
  let fake = createFakeGateway();
  fake.program('OpenSession', 'sess-1');
  setGateway(fake);
  const id = await openSessionRpc('conn-1');
  assert(id === 'sess-1', 'openSessionRpc returns the id string returned by OpenSession');
  const openCall = fake.calls.find((c) => c.method === 'OpenSession');
  assert(!!openCall && openCall.args[0] === 'conn-1', 'openSessionRpc forwards connectionId');

  // openSessionRpc: propagates RPC errors (no swallow, no handleError)
  fake = createFakeGateway();
  fake.program('OpenSession', () => { throw new Error('open failed'); });
  setGateway(fake);
  lastError.set(null);
  let threw: unknown = null;
  try { await openSessionRpc('conn-1'); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'open failed', 'openSessionRpc propagates the raw RPC error');
  assert(get(lastError) === null, 'openSessionRpc does not call handleError itself');

  // closeSessionRpc: does NOT swallow 'session not found' — it propagates,
  // in contrast with the still-in-place closeSession orchestration wrapper
  // which swallows this specific error.
  fake = createFakeGateway();
  fake.program('CloseSession', () => { throw new Error('session not found'); });
  setGateway(fake);
  lastError.set(null);
  threw = null;
  try { await closeSessionRpc('sess-1'); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'session not found', 'closeSessionRpc propagates "session not found" instead of swallowing it');
  assert(get(lastError) === null, 'closeSessionRpc does not call handleError itself (no side effects at all)');

  // closeSessionRpc: propagates other RPC errors too
  fake = createFakeGateway();
  fake.program('CloseSession', () => { throw new Error('other failure'); });
  setGateway(fake);
  threw = null;
  try { await closeSessionRpc('sess-1'); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'other failure', 'closeSessionRpc propagates arbitrary RPC errors');

  // closeSessionRpc: success forwards sessionId
  fake = createFakeGateway();
  fake.program('CloseSession', undefined);
  setGateway(fake);
  await closeSessionRpc('sess-9');
  const closeCall = fake.calls.find((c) => c.method === 'CloseSession');
  assert(!!closeCall && closeCall.args[0] === 'sess-9', 'closeSessionRpc forwards sessionId');

  // reportEmbedViewport: no-op when method absent
  fake = createFakeGateway();
  const noViewport = new Proxy(fake, { get(t, p: string) { return p === 'ReportEmbedViewport' ? undefined : (t as any)[p]; } });
  setGateway(noViewport as any);
  await reportEmbedViewport('sess-1', 100, 200, 1.5);

  // reportEmbedViewport: forwards args; swallows failures via handleError
  fake = createFakeGateway();
  fake.program('ReportEmbedViewport', undefined);
  setGateway(fake);
  await reportEmbedViewport('sess-1', 100, 200, 1.5);
  const viewportCall = fake.calls.find((c) => c.method === 'ReportEmbedViewport');
  assert(!!viewportCall && viewportCall.args[0] === 'sess-1' && viewportCall.args[1] === 100 && viewportCall.args[2] === 200 && viewportCall.args[3] === 1.5, 'reportEmbedViewport forwards all args');

  fake = createFakeGateway();
  fake.program('ReportEmbedViewport', () => { throw new Error('viewport failed'); });
  setGateway(fake);
  lastError.set(null);
  await reportEmbedViewport('sess-1', 1, 2, 3);
  assert(get(lastError)?.message === 'Report embed viewport: viewport failed', 'reportEmbedViewport reports failure via handleError instead of throwing');

  // reportEmbedActivity: forwards args; swallows failures via handleError
  fake = createFakeGateway();
  fake.program('ReportEmbedActivity', undefined);
  setGateway(fake);
  await reportEmbedActivity('sess-1', true);
  const activityCall = fake.calls.find((c) => c.method === 'ReportEmbedActivity');
  assert(!!activityCall && activityCall.args[0] === 'sess-1' && activityCall.args[1] === true, 'reportEmbedActivity forwards args');

  fake = createFakeGateway();
  fake.program('ReportEmbedActivity', () => { throw new Error('activity failed'); });
  setGateway(fake);
  lastError.set(null);
  await reportEmbedActivity('sess-1', false);
  assert(get(lastError)?.message === 'Report embed activity: activity failed', 'reportEmbedActivity reports failure via handleError instead of throwing');

  // getPlatform: returns 'unknown' when gateway absent
  setGateway(null as any);
  const p1 = await getPlatform();
  assert(p1 === 'unknown', 'getPlatform returns unknown when gateway absent');

  // getPlatform: returns RPC result
  fake = createFakeGateway();
  fake.program('GetPlatform', 'linux');
  setGateway(fake);
  const p2 = await getPlatform();
  assert(p2 === 'linux', 'getPlatform returns the RPC result');

  // getPlatform: swallows errors, returns 'unknown', no lastError
  fake = createFakeGateway();
  fake.program('GetPlatform', () => { throw new Error('platform failed'); });
  setGateway(fake);
  lastError.set(null);
  const p3 = await getPlatform();
  assert(p3 === 'unknown', 'getPlatform falls back to unknown on RPC failure');
  assert(get(lastError) === null, 'getPlatform silently swallows errors without setting lastError');

  // resolveHostKeyRpc: forwards args, clears pendingHostKey on success
  fake = createFakeGateway();
  fake.program('ResolveHostKey', undefined);
  setGateway(fake);
  pendingHostKey.set({ sessionId: 's', host: 'h', fingerprint: 'f', keyType: 'k' } as any);
  await resolveHostKeyRpc('sess-1', 'accept', 'host1', 'key1');
  const rhkCall = fake.calls.find((c) => c.method === 'ResolveHostKey');
  assert(!!rhkCall && rhkCall.args[0] === 'sess-1' && rhkCall.args[1] === 'accept' && rhkCall.args[2] === 'host1' && rhkCall.args[3] === 'key1', 'resolveHostKeyRpc forwards args');
  assert(get(pendingHostKey) === null, 'resolveHostKeyRpc clears pendingHostKey on success');

  // resolveHostKeyRpc: swallows failure via handleError, does not clear pendingHostKey
  fake = createFakeGateway();
  fake.program('ResolveHostKey', () => { throw new Error('resolve failed'); });
  setGateway(fake);
  lastError.set(null);
  pendingHostKey.set({ sessionId: 's', host: 'h', fingerprint: 'f', keyType: 'k' } as any);
  await resolveHostKeyRpc('sess-1', 'accept', 'host1', 'key1');
  assert(get(lastError)?.message === 'Resolve host key: resolve failed', 'resolveHostKeyRpc reports failure via handleError');
  assert(get(pendingHostKey) !== null, 'resolveHostKeyRpc does not clear pendingHostKey on failure');

  console.log('sessions.test.ts passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
