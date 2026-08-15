import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import { getPeerTrust, removePeerTrust } from './peerTrust';
import { lastError, pendingPeerTrust, enqueuePeerTrust, dropPeerTrust, type PeerTrustEvent } from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

function question(sessionId: string, fingerprint = 'SHA256:a'): PeerTrustEvent {
  return { sessionId, subject: 'h:1', fingerprint, mismatch: false };
}

async function run() {
  // There is deliberately no addPeerTrust wrapper. Recording trust is reachable only through
  // resolvePeerTrustRpc, which answers a question the backend itself raised about material it
  // observed. A binding taking a subject and material from the frontend would let anything holding
  // the bridge pre-seed trust for a peer the user never connected to.
  let fake = createFakeGateway();
  fake.program('GetPeerTrust', [
    { scope: 'com.p', subject: 'h:3389', fingerprint: 'SHA256:a', addedAt: '2026-01-01T00:00:00Z' },
  ]);
  setGateway(fake);
  lastError.set(null);

  const entries = await getPeerTrust();
  assert(entries.length === 1 && entries[0].subject === 'h:3389', 'getPeerTrust returns what the backend listed');
  assert(!('material' in entries[0]), 'the listing carries no key material');
  assert(get(lastError) === null, 'no error reported for a successful listing');

  // A null answer is an empty list, not a crash: the backend omits the field when nothing is stored.
  fake = createFakeGateway();
  fake.program('GetPeerTrust', null);
  setGateway(fake);
  assert((await getPeerTrust()).length === 0, 'getPeerTrust treats a null answer as an empty list');

  // Both halves of the coordinate are sent: an entry is keyed by (scope, subject), and deleting by
  // subject alone would revoke every plugin's trust in that peer.
  fake = createFakeGateway();
  fake.program('RemovePeerTrust', undefined);
  setGateway(fake);
  await removePeerTrust('com.p', 'h:3389');
  const call = fake.calls.find((c) => c.method === 'RemovePeerTrust');
  assert(!!call && call.args[0] === 'com.p' && call.args[1] === 'h:3389', 'removePeerTrust forwards scope and subject');

  // Failures surface to the user rather than vanishing into the console.
  fake = createFakeGateway();
  fake.program('RemovePeerTrust', () => { throw new Error('boom'); });
  setGateway(fake);
  lastError.set(null);
  await removePeerTrust('com.p', 'h:3389');
  assert(get(lastError) !== null, 'a failed removal reports an error');

  fake = createFakeGateway();
  fake.program('GetPeerTrust', () => { throw new Error('locked'); });
  setGateway(fake);
  lastError.set(null);
  assert((await getPeerTrust()).length === 0, 'a failed listing falls back to an empty list');
  assert(get(lastError) !== null, 'a failed listing reports an error');

  // --- the queue ---

  // Two sessions can each be stopped by a question. With a single slot the second overwrote the
  // first, and the first session waited forever on a dialog that no longer existed.
  pendingPeerTrust.set([]);
  enqueuePeerTrust(question('s1'));
  enqueuePeerTrust(question('s2'));
  assert(get(pendingPeerTrust).length === 2, 'a second session does not evict the first question');
  assert(get(pendingPeerTrust)[0].sessionId === 's1', 'the oldest question stays at the head');

  // A repeat for the same session replaces its entry: a plugin retrying its handshake re-raises
  // the same question, and two identical dialogs teach the user to dismiss them.
  enqueuePeerTrust(question('s1', 'SHA256:b'));
  const queue = get(pendingPeerTrust);
  assert(queue.length === 2, 'a repeat for a queued session does not add a second entry');
  assert(queue.filter((q) => q.sessionId === 's1')[0].fingerprint === 'SHA256:b', 'the repeat carries the newest fingerprint');

  // Answering removes only that session's question.
  dropPeerTrust('s2');
  assert(get(pendingPeerTrust).length === 1, 'answering one question removes exactly one');
  assert(get(pendingPeerTrust)[0].sessionId === 's1', 'answering one question leaves the other alone');

  console.log('peerTrust.test.ts: all assertions passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
