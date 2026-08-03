import { setGateway } from '../backend/context';
import { createFakeGateway } from '../backend/fakeGateway';
import { unlockVaultRpc, lockVaultRpc } from './vault';

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

  console.log('vault.test.ts passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
