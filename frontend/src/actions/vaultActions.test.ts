import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import { unlockVault, lockVault } from './vaultActions';
import {
  folders, connections, sessions, identities, vaultUnlocked,
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
  lastError.set(null);
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
    fake.program('GetIdentities', []);
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
    const expectedSubset = ['UnlockVault', 'GetPlatform', 'GetFolders', 'GetAllConnections', 'GetIdentities', 'GetPluginConnectionProtocols', 'GetSettings'];
    for (const m of expectedSubset) {
      assert(methods.includes(m), `unlockVault RPC sequence includes ${m}`);
    }
    assert(methods[0] === 'UnlockVault', 'UnlockVault is the first RPC call');
    assert(methods[1] === 'GetPlatform', 'GetPlatform is the second RPC call');
    assert(methods.indexOf('GetFolders') < methods.indexOf('GetAllConnections'), 'GetFolders happens before GetAllConnections');
    assert(methods.indexOf('GetAllConnections') < methods.indexOf('GetIdentities'), 'GetAllConnections happens before GetIdentities');
    assert(methods.indexOf('GetIdentities') < methods.indexOf('GetPluginConnectionProtocols'), 'GetIdentities happens before protocol refresh');
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

  console.log('vaultActions.test passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
