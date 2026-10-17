import { get } from 'svelte/store';
import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import { connectSync, identityVouched, refreshAfterSync, type ConnectSyncForm } from './syncActions';
import { syncRevision } from '../stores/syncState';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

const fp = '91796 59138 05751 85871 52528 03481 07460 08300 23148 85620 11178 34775';
const other = fp.replace('91796', '91797');
const probe = { url: 'https://sync.example:8443', identityKey: 'ab'.repeat(32), fingerprint: fp };
const form = (over: Partial<ConnectSyncForm> = {}): ConnectSyncForm => ({
  address: 'https://sync.example:8443', username: 'alice', password: 'pw', invite: '',
  expectedFingerprint: '', confirmed: false, replace: false, ...over,
});

function gateway(joinNeeded = false) {
  const fake = createFakeGateway();
  fake.program('EnableSync', { joinNeeded });
  fake.program('JoinSync', 'vault.age.sync-join-1.bak');
  fake.program('GetSyncStatus', { enabled: true, running: true, state: 'idle', held: 0, quarantined: 0 });
  setGateway(fake);
  return fake;
}

async function run() {
  // Nothing is sent before the server's identity is vouched for.
  for (const [name, f] of [
    ['nobody compared it', form()],
    ['the link names another server', form({ address: `https://sync.example:8443/#fp=${other.replace(/ /g, '-')}` })],
    ['the pasted fingerprint differs', form({ expectedFingerprint: other, confirmed: true })],
  ] as const) {
    const fake = gateway();
    const err = await connectSync(f, probe);
    assert(err !== '', `${name}: connected anyway`);
    assert(!fake.calls.some((c) => c.method === 'EnableSync'), `${name}: credentials reached the server`);
  }
  assert(identityVouched(form({ expectedFingerprint: other, confirmed: true }), probe) !== '',
    'a fingerprint that was supplied and differs outranks the person ticking the box');

  // The link vouches by itself: no box to tick, one click.
  {
    const fake = gateway();
    const err = await connectSync(form({ address: `https://sync.example:8443/#fp=${fp.replace(/ /g, '-')}` }), probe);
    assert(err === '', `link sign-in: ${err}`);
    const call = fake.calls.find((c) => c.method === 'EnableSync');
    const req = call?.args[0] as { serverUrl: string; register: boolean; identityKey: string };
    assert(req.serverUrl === 'https://sync.example:8443', `server url = ${req.serverUrl}; the fragment is not part of it`);
    assert(!req.register && req.identityKey === probe.identityKey, 'no invite signs in, pinning the probed key');
  }

  // An invite code makes it a new account.
  {
    const fake = gateway();
    await connectSync(form({ confirmed: true, invite: ' CODE ' }), probe);
    const req = fake.calls.find((c) => c.method === 'EnableSync')?.args[0] as { register: boolean; invite: string };
    assert(req.register && req.invite === 'CODE', 'an invite registers, trimmed');
  }

  // Both sides hold data: merge without asking, unless the person chose replace beforehand.
  for (const replace of [false, true]) {
    const fake = gateway(true);
    const err = await connectSync(form({ confirmed: true, replace }), probe);
    const choice = fake.calls.find((c) => c.method === 'JoinSync')?.args[0];
    assert(err === '' && choice === (replace ? 'replace' : 'merge'), `join choice = ${String(choice)} for replace=${replace}`);
  }

  // A probe taken for another address is never pinned.
  {
    const fake = gateway();
    const err = await connectSync(form({ address: 'https://elsewhere.example', confirmed: true }), probe);
    assert(err !== '' && !fake.calls.some((c) => c.method === 'EnableSync'), 'a stale probe was used');
  }

  // A refresh after sync re-reads the vault and tells every open list to reload.
  {
    const fake = createFakeGateway();
    setGateway(fake);
    const before = get(syncRevision);
    await refreshAfterSync();
    assert(get(syncRevision) === before + 1, 'open lists are told the vault changed');
    assert(fake.calls.some((c) => c.method === 'GetAllConnections'), 'the connection tree is re-read');
  }

  // A burst of refreshes is one refresh: every open list re-verifies against the server on reload,
  // and one reload per call tripped the server's rate limit after a join.
  {
    const fake = createFakeGateway();
    setGateway(fake);
    const before = get(syncRevision);
    await Promise.all([refreshAfterSync(), refreshAfterSync(), refreshAfterSync()]);
    const reads = fake.calls.filter((c) => c.method === 'GetAllConnections').length;
    assert(reads === 1 && get(syncRevision) === before + 1, `${reads} reads, revision +${get(syncRevision) - before}; a burst is folded into one`);
  }
  console.log('syncConnect tests passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
