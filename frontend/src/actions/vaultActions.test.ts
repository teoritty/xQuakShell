import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import {
  unlockVault, lockVault, createVault, initVaultGate,
  completeRecoveryReset, acknowledgeRecoveryKey, regenerateRecoveryKey,
} from './vaultActions';
import {
  folders, connections, sessions, identities, vaultUnlocked, vaultExists,
  pendingRecoveryKey, recoveryResetRequired,
  lastError,
  type Folder, type Connection,
} from '../stores/appState';
import { get } from 'svelte/store';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

function reset() {
  folders.set([]);
  connections.set([]);
  sessions.set([]);
  identities.set([]);
  vaultUnlocked.set(false);
  vaultExists.set(null);
  pendingRecoveryKey.set(null);
  recoveryResetRequired.set(false);
  lastError.set(null);
}

// Programs the warmup RPCs a successful open triggers, so a test can assert on what else happened.
function programWarmup(fake: ReturnType<typeof createFakeGateway>) {
  fake.program('GetPlatform', 'linux');
  fake.program('GetFolders', [] as Folder[]);
  fake.program('GetAllConnections', [] as Connection[]);
  fake.program('GetKeys', []);
  fake.program('GetPluginConnectionProtocols', []);
}

async function run() {
  // --- unlockVault -----------------------------------------------------

  // Success: UnlockVault first, GetPlatform second, then folders/connections/
  // identities/protocols refresh in order, then appearance settings applied.
  {
    reset();
    const fake = createFakeGateway();
    fake.program('UnlockVault', undefined);
    fake.program('GetPlatform', 'linux');
    fake.program('GetFolders', [{ id: 'f1', name: 'F', parentId: '', order: 0 }] as Folder[]);
    fake.program('GetAllConnections', [{ id: 'c1', folderId: 'f1', name: 'C', host: 'h', port: 22, order: 0 }] as Connection[]);
    fake.program('GetKeys', []);
    fake.program('GetPluginConnectionProtocols', []);
    // GetSettings deliberately unprogrammed (see library.char.test.ts comment):
    // resolves undefined, getSettings() throws internally and is caught,
    // applyAppearanceSettings's `if (!s) return;` short-circuits.
    setGateway(fake);

    await unlockVault('pw');

    assert(get(vaultUnlocked) === true, 'unlockVault sets vaultUnlocked to true');
    assert(get(folders).length === 1 && get(folders)[0].id === 'f1', 'unlockVault populates folders');
    assert(get(connections).length === 1 && get(connections)[0].id === 'c1', 'unlockVault populates connections');
    assert(get(identities).length === 0, 'unlockVault populates identities');

    const methods = fake.calls.map((c) => c.method);
    const expectedSubset = ['UnlockVault', 'GetPlatform', 'GetFolders', 'GetAllConnections', 'GetKeys', 'GetPluginConnectionProtocols', 'GetSettings'];
    for (const m of expectedSubset) {
      assert(methods.includes(m), `unlockVault RPC sequence includes ${m}`);
    }
    assert(methods[0] === 'UnlockVault', 'UnlockVault is the first RPC call');
    assert(methods[1] === 'GetPlatform', 'GetPlatform is the second RPC call');
    assert(methods.indexOf('GetFolders') < methods.indexOf('GetAllConnections'), 'GetFolders happens before GetAllConnections');
    assert(methods.indexOf('GetAllConnections') < methods.indexOf('GetKeys'), 'GetAllConnections happens before GetKeys');
    assert(methods.indexOf('GetKeys') < methods.indexOf('GetPluginConnectionProtocols'), 'GetKeys happens before protocol refresh');
  }

  // Missing gateway: original guards before ANY store mutation and returns
  // silently (no UnlockVault call, vaultUnlocked untouched, no error toast).
  {
    reset();
    setGateway(null);

    await unlockVault('pw');

    assert(get(vaultUnlocked) === false, 'unlockVault does not touch vaultUnlocked when gateway is missing');
    assert(get(folders).length === 0, 'unlockVault does not touch folders when gateway is missing');
    assert(get(connections).length === 0, 'unlockVault does not touch connections when gateway is missing');
    assert(get(identities).length === 0, 'unlockVault does not touch identities when gateway is missing');
    assert(get(lastError) === null, 'unlockVault does not set lastError when gateway is missing');
  }

  // --- createVault -------------------------------------------------------

  // Success: same warmup as unlockVault, in the same order, because both share
  // warmupAfterVaultOpened. CreateVault simply takes UnlockVault's place.
  {
    reset();
    const fake = createFakeGateway();
    fake.program('CreateVault', undefined);
    fake.program('GetPlatform', 'linux');
    fake.program('GetFolders', [] as Folder[]);
    fake.program('GetAllConnections', [] as Connection[]);
    fake.program('GetKeys', []);
    fake.program('GetPluginConnectionProtocols', []);
    setGateway(fake);

    await createVault('a-good-master-password');

    assert(get(vaultUnlocked) === true, 'createVault sets vaultUnlocked to true');
    assert(get(vaultExists) === true, 'createVault marks the vault as existing');

    const methods = fake.calls.map((c) => c.method);
    assert(methods[0] === 'CreateVault', 'CreateVault is the first RPC call');
    assert(methods[1] === 'GetPlatform', 'GetPlatform is the second RPC call');
    for (const m of ['GetFolders', 'GetAllConnections', 'GetKeys', 'GetPluginConnectionProtocols', 'GetSettings']) {
      assert(methods.includes(m), `createVault RPC sequence includes ${m}`);
    }
    assert(methods.indexOf('GetFolders') < methods.indexOf('GetAllConnections'), 'GetFolders happens before GetAllConnections');
    assert(methods.indexOf('GetAllConnections') < methods.indexOf('GetKeys'), 'GetAllConnections happens before GetKeys');
    assert(methods.indexOf('GetKeys') < methods.indexOf('GetPluginConnectionProtocols'), 'GetKeys happens before protocol refresh');

    const call = fake.calls.find((c) => c.method === 'CreateVault');
    assert(!!call && call.args[0] === 'a-good-master-password', 'createVault forwards the master password');
  }

  // The RPC error propagates so the create screen can show it next to the field.
  {
    reset();
    const fake = createFakeGateway();
    fake.program('CreateVault', () => {
      throw new Error('vault already exists');
    });
    setGateway(fake);

    let threw: unknown = null;
    try { await createVault('a-good-master-password'); } catch (e) { threw = e; }

    assert(threw instanceof Error && threw.message === 'vault already exists', 'createVault propagates the RPC error');
    assert(get(vaultUnlocked) === false, 'a failed createVault leaves vaultUnlocked alone');
    assert(get(vaultExists) === null, 'a failed createVault leaves vaultExists unanswered');
  }

  // Missing gateway: same silent guard as unlockVault.
  {
    reset();
    setGateway(null);

    await createVault('a-good-master-password');

    assert(get(vaultUnlocked) === false, 'createVault does not touch vaultUnlocked when gateway is missing');
    assert(get(vaultExists) === null, 'createVault does not touch vaultExists when gateway is missing');
    assert(get(lastError) === null, 'createVault does not set lastError when gateway is missing');
  }

  // --- initVaultGate -----------------------------------------------------

  for (const exists of [true, false]) {
    reset();
    const fake = createFakeGateway();
    fake.program('VaultExists', exists);
    setGateway(fake);

    await initVaultGate();

    assert(get(vaultExists) === exists, `initVaultGate reports VaultExists === ${exists}`);
  }

  // Missing gateway: the create screen is the safe default for a backendless run.
  {
    reset();
    setGateway(null);

    await initVaultGate();

    assert(get(vaultExists) === false, 'initVaultGate falls back to false when gateway is missing');
  }

  // --- lockVault ---------------------------------------------------------

  // LockVault RPC error is swallowed (via handleError) but folders/
  // connections/sessions/identities/vaultUnlocked are still cleared.
  {
    reset();
    folders.set([{ id: 'f1', name: 'F', parentId: '', order: 0 }]);
    connections.set([{ id: 'c1', folderId: 'f1', name: 'C', host: 'h', port: 22, order: 0 }]);
    sessions.set([{ sessionId: 's1', connectionId: 'c1', connectionName: 'C', state: 'ready', errorMessage: '' } as any]);
    identities.set([{ id: 'i1', comment: '', keyType: 'ed25519' } as any]);
    vaultUnlocked.set(true);
    vaultExists.set(true);

    const fake = createFakeGateway();
    fake.program('LockVault', () => {
      throw new Error('lock rpc failed');
    });
    setGateway(fake);

    await lockVault();

    assert(get(folders).length === 0, 'lockVault empties folders even when LockVault RPC throws');
    assert(get(connections).length === 0, 'lockVault empties connections even when LockVault RPC throws');
    assert(get(sessions).length === 0, 'lockVault empties sessions even when LockVault RPC throws');
    assert(get(identities).length === 0, 'lockVault empties identities even when LockVault RPC throws');
    assert(get(vaultUnlocked) === false, 'lockVault sets vaultUnlocked to false even when LockVault RPC throws');
    assert(get(vaultExists) === true, 'lockVault leaves vaultExists alone: a locked vault still exists');
    const err = get(lastError);
    assert(err !== null && err.message === 'Lock vault: lock rpc failed', 'lockVault surfaces the LockVault RPC error via handleError instead of propagating it');
  }

  // Missing gateway: original guards before ANY store mutation and returns
  // silently (no LockVault call, stores untouched, no error toast).
  {
    reset();
    folders.set([{ id: 'f1', name: 'F', parentId: '', order: 0 }]);
    connections.set([{ id: 'c1', folderId: 'f1', name: 'C', host: 'h', port: 22, order: 0 }]);
    sessions.set([{ sessionId: 's1', connectionId: 'c1', connectionName: 'C', state: 'ready', errorMessage: '' } as any]);
    identities.set([{ id: 'i1', comment: '', keyType: 'ed25519' } as any]);
    vaultUnlocked.set(true);
    setGateway(null);

    await lockVault();

    assert(get(folders).length === 1, 'lockVault does not touch folders when gateway is missing');
    assert(get(connections).length === 1, 'lockVault does not touch connections when gateway is missing');
    assert(get(sessions).length === 1, 'lockVault does not touch sessions when gateway is missing');
    assert(get(identities).length === 1, 'lockVault does not touch identities when gateway is missing');
    assert(get(vaultUnlocked) === true, 'lockVault does not touch vaultUnlocked when gateway is missing');
    assert(get(lastError) === null, 'lockVault does not set lastError when gateway is missing');
  }

  // --- recovery key ------------------------------------------------------

  // A recovery unlock must stop at the reset screen. Warming the stores up would put the user in a
  // running application whose only credential is one they were told to keep on paper.
  {
    reset();
    const fake = createFakeGateway();
    fake.program('UnlockVault', { method: 'recovery' });
    programWarmup(fake);
    setGateway(fake);

    await unlockVault('AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA-AAAA');

    assert(get(recoveryResetRequired) === true, 'a recovery unlock asks for a new password');
    assert(get(vaultUnlocked) === false, 'a recovery unlock does not open the application');
    const methods = fake.calls.map((c) => c.method);
    assert(!methods.includes('GetPlatform'), 'a recovery unlock does not warm the stores up');
    assert(get(pendingRecoveryKey) === null, 'a recovery unlock hands back no key of its own');
  }

  // A password unlock of a vault that was just converted from the old format carries a first key.
  {
    reset();
    const fake = createFakeGateway();
    fake.program('UnlockVault', { method: 'password', recoveryKey: 'AAAA-BBBB' });
    programWarmup(fake);
    setGateway(fake);

    await unlockVault('pw');

    assert(get(vaultUnlocked) === true, 'a password unlock still opens the application');
    assert(get(pendingRecoveryKey) === 'AAAA-BBBB', 'the key minted during conversion is shown');
    assert(get(recoveryResetRequired) === false, 'a password unlock asks for no reset');
  }

  // Creating a vault hands over its one-time key in the same call.
  {
    reset();
    const fake = createFakeGateway();
    fake.program('CreateVault', { key: 'CCCC-DDDD' });
    programWarmup(fake);
    setGateway(fake);

    await createVault('a-good-master-password');

    assert(get(pendingRecoveryKey) === 'CCCC-DDDD', 'createVault shows the key it was given');
    assert(get(vaultUnlocked) === true, 'createVault still opens the application');
  }

  // Finishing the reset is what starts the application, and it issues a fresh key.
  {
    reset();
    recoveryResetRequired.set(true);
    const fake = createFakeGateway();
    fake.program('CompleteRecoveryReset', { key: 'EEEE-FFFF' });
    programWarmup(fake);
    setGateway(fake);

    await completeRecoveryReset('a-brand-new-password');

    assert(get(recoveryResetRequired) === false, 'the reset screen is done');
    assert(get(pendingRecoveryKey) === 'EEEE-FFFF', 'the reset issues a new key');
    assert(get(vaultUnlocked) === true, 'the application starts only after the reset');
    const call = fake.calls.find((c) => c.method === 'CompleteRecoveryReset');
    assert(!!call && call.args[0] === 'a-brand-new-password', 'the new password is forwarded');
  }

  // Regenerating from settings shows the new key without touching the open session.
  {
    reset();
    vaultUnlocked.set(true);
    const fake = createFakeGateway();
    fake.program('IssueRecoveryKey', { key: 'GGGG-HHHH' });
    setGateway(fake);

    await regenerateRecoveryKey('the-master-password');

    assert(get(pendingRecoveryKey) === 'GGGG-HHHH', 'regenerating shows the new key');
    assert(get(vaultUnlocked) === true, 'regenerating leaves the session alone');
    const call = fake.calls.find((c) => c.method === 'IssueRecoveryKey');
    assert(!!call && call.args[0] === 'the-master-password', 'the master password is forwarded');
  }

  // Done tells the backend first. The store is only cleared once the backend has dropped its copy,
  // so a failure leaves the key on screen rather than wiping the display of something still held.
  {
    reset();
    pendingRecoveryKey.set('IIII-JJJJ');
    const fake = createFakeGateway();
    fake.program('AcknowledgeRecoveryKey', () => {
      throw new Error('bridge is gone');
    });
    setGateway(fake);

    await acknowledgeRecoveryKey();

    assert(get(pendingRecoveryKey) === 'IIII-JJJJ', 'a failed acknowledgement leaves the key visible');
    const err = get(lastError);
    assert(err !== null && err.message.includes('bridge is gone'), 'the failure is surfaced');
  }

  {
    reset();
    pendingRecoveryKey.set('KKKK-LLLL');
    const fake = createFakeGateway();
    fake.program('AcknowledgeRecoveryKey', undefined);
    setGateway(fake);

    await acknowledgeRecoveryKey();

    assert(get(pendingRecoveryKey) === null, 'a successful acknowledgement clears the key');
    assert(fake.calls.some((c) => c.method === 'AcknowledgeRecoveryKey'), 'the backend was told to forget it');
  }

  // Locking clears both, because a key still on screen when the vault locks is one nobody can act
  // on any more and the backend has already dropped its own copy.
  {
    reset();
    pendingRecoveryKey.set('MMMM-NNNN');
    recoveryResetRequired.set(true);
    vaultUnlocked.set(true);
    const fake = createFakeGateway();
    fake.program('LockVault', undefined);
    setGateway(fake);

    await lockVault();

    assert(get(pendingRecoveryKey) === null, 'locking clears the key on screen');
    assert(get(recoveryResetRequired) === false, 'locking clears the pending reset');
  }

  console.log('vaultActions.test passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
