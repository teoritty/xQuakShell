import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import { unlockVaultRpc, lockVaultRpc, createVaultRpc, vaultExistsRpc } from './vault';

function assert(c: boolean, m: string) { if (!c) throw new Error(m); }

async function run() {
  // unlockVaultRpc: forwards masterPassword to UnlockVault
  let fake = createFakeGateway();
  fake.program('UnlockVault', undefined);
  setGateway(fake);
  await unlockVaultRpc('hunter2');
  const call = fake.calls.find((c) => c.method === 'UnlockVault');
  assert(!!call && call.args[0] === 'hunter2', 'unlockVaultRpc forwards masterPassword to UnlockVault');

  // unlockVaultRpc: propagates RPC errors (no swallow, no handleError)
  fake = createFakeGateway();
  fake.program('UnlockVault', () => { throw new Error('bad password'); });
  setGateway(fake);
  let threw: unknown = null;
  try { await unlockVaultRpc('wrong'); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'bad password', 'unlockVaultRpc propagates the raw RPC error');

  // lockVaultRpc: calls LockVault
  fake = createFakeGateway();
  fake.program('LockVault', undefined);
  setGateway(fake);
  await lockVaultRpc();
  assert(fake.calls.some((c) => c.method === 'LockVault'), 'lockVaultRpc calls LockVault');

  // lockVaultRpc: propagates RPC errors (no swallow, no handleError)
  fake = createFakeGateway();
  fake.program('LockVault', () => { throw new Error('lock rpc failed'); });
  setGateway(fake);
  threw = null;
  try { await lockVaultRpc(); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'lock rpc failed', 'lockVaultRpc propagates the raw RPC error');

  // createVaultRpc: forwards masterPassword to CreateVault
  fake = createFakeGateway();
  fake.program('CreateVault', undefined);
  setGateway(fake);
  await createVaultRpc('a-good-master-password');
  const createCall = fake.calls.find((c) => c.method === 'CreateVault');
  assert(!!createCall && createCall.args[0] === 'a-good-master-password', 'createVaultRpc forwards masterPassword to CreateVault');

  // createVaultRpc: propagates RPC errors (no swallow, no handleError)
  fake = createFakeGateway();
  fake.program('CreateVault', () => { throw new Error('vault already exists'); });
  setGateway(fake);
  threw = null;
  try { await createVaultRpc('a-good-master-password'); } catch (e) { threw = e; }
  assert(threw instanceof Error && threw.message === 'vault already exists', 'createVaultRpc propagates the raw RPC error');

  // createVaultRpc: silent no-op without a gateway
  setGateway(null);
  threw = null;
  try { await createVaultRpc('a-good-master-password'); } catch (e) { threw = e; }
  assert(threw === null, 'createVaultRpc is a silent no-op when the gateway is missing');

  // vaultExistsRpc: returns whatever the backend reports
  for (const exists of [true, false]) {
    fake = createFakeGateway();
    fake.program('VaultExists', exists);
    setGateway(fake);
    assert(await vaultExistsRpc() === exists, `vaultExistsRpc returns ${exists}`);
  }

  // vaultExistsRpc: false without a gateway, so a backendless run offers create
  setGateway(null);
  assert(await vaultExistsRpc() === false, 'vaultExistsRpc returns false when the gateway is missing');

  console.log('vault.test.ts passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
