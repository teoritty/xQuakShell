import { get } from 'svelte/store';
import { setGateway, setRuntime } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import { enableSync, joinSync, probeSyncServer, refreshSyncStatus, confirmSyncHeld, syncNow } from './syncActions';
import { applySyncRound, offSyncStatus, syncStatus } from '../stores/syncState';
import { subscribeSyncEvents } from '../events/syncEvents';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

const enabled = {
  enabled: true, running: true, state: 'idle', held: 0, quarantined: 0,
  serverUrl: 'https://sync.example', username: 'alice', fingerprint: '00001',
};

async function run() {
  // Enabling sends the request as given and refreshes the status afterwards.
  {
    syncStatus.set(offSyncStatus);
    const fake = createFakeGateway();
    fake.program('EnableSync', { joinNeeded: true });
    fake.program('GetSyncStatus', enabled);
    setGateway(fake);
    const req = { serverUrl: 'https://sync.example', identityKey: 'ab', username: 'alice', password: 'pw', invite: '', register: false };
    const res = await enableSync(req);
    assert(res.error === '' && res.joinNeeded, 'a login that found data on both sides reports joinNeeded');
    const call = fake.calls.find((c) => c.method === 'EnableSync');
    assert(call !== undefined && (call.args[0] as typeof req).password === 'pw', 'the request reaches the backend unchanged');
    assert(get(syncStatus).enabled && get(syncStatus).username === 'alice', 'the status is refreshed after enabling');
  }

  // A refusal comes back as the message to show, and nothing throws.
  {
    const fake = createFakeGateway();
    fake.program('EnableSync', () => {
      throw new Error('Wrong username or sync password.');
    });
    fake.program('GetSyncStatus', offSyncStatus);
    setGateway(fake);
    const res = await enableSync({ serverUrl: 'https://x', identityKey: 'ab', username: 'a', password: 'b', invite: '', register: false });
    assert(res.error === 'Wrong username or sync password.' && !res.joinNeeded, 'the backend message is returned for the form');
  }

  // Probing trims the address and passes the identity through for the fingerprint check.
  {
    const fake = createFakeGateway();
    fake.program('ProbeSyncServer', { identityKey: 'aa', fingerprint: '11111 22222' });
    setGateway(fake);
    const res = await probeSyncServer('  https://sync.example  ');
    assert(res.probe?.fingerprint === '11111 22222', 'probe returns the fingerprint');
    assert(fake.calls[0].args[0] === 'https://sync.example', 'the address is trimmed before it is sent');
  }

  // Join returns the backup name so the user knows where the copy is.
  {
    const fake = createFakeGateway();
    fake.program('JoinSync', 'vault.age.sync-join-1.bak');
    fake.program('GetSyncStatus', enabled);
    setGateway(fake);
    const res = await joinSync('merge');
    assert(res.backup === 'vault.age.sync-join-1.bak' && res.error === '', 'join reports the backup');
    assert(fake.calls[0].args[0] === 'merge', 'the choice reaches the backend');
  }

  // The master password is passed only to the confirmation it is for.
  {
    const fake = createFakeGateway();
    fake.program('ConfirmSyncHeld', undefined);
    fake.program('GetSyncStatus', enabled);
    setGateway(fake);
    assert((await confirmSyncHeld('rec', 'master')) === '', 'confirmation succeeds');
    const c = fake.calls.find((x) => x.method === 'ConfirmSyncHeld');
    assert(c !== undefined && c.args[0] === 'rec' && c.args[1] === 'master', 'record and password reach the backend');
  }

  // A round's status keeps the account fields the event does not carry.
  {
    syncStatus.set({ ...enabled, state: 'idle' } as any);
    applySyncRound({ enabled: false, running: true, state: 'failed', message: 'down', held: 1, quarantined: 2 });
    const st = get(syncStatus);
    assert(st.state === 'failed' && st.message === 'down' && st.held === 1, 'the round is applied');
    assert(st.username === 'alice' && st.enabled, 'the account is kept across round events');
  }

  // Events reach the store through the runtime subscription.
  {
    let handler: ((d: unknown) => void) | undefined;
    setRuntime({ EventsOn: (name: string, cb: (d: any) => void) => { if (name === 'SyncStatus') handler = cb; }, BrowserOpenURL: () => {} });
    syncStatus.set(enabled as any);
    subscribeSyncEvents();
    assert(handler !== undefined, 'subscribed to SyncStatus');
    handler!({ running: true, state: 'stopped', held: 0, quarantined: 0, message: 'verification failed' });
    assert(get(syncStatus).state === 'stopped', 'a stopped round reaches the indicator');
    handler!({ junk: true });
    assert(get(syncStatus).state === 'stopped', 'a malformed event is ignored');
  }

  // Without a gateway nothing throws.
  {
    setGateway(null);
    await refreshSyncStatus();
    assert((await syncNow()) === '', 'no gateway: no error, nothing done');
  }

  console.log('syncActions: all assertions passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
