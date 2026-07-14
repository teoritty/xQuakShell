// Atomic vault RPC wrappers. Matches the original stores/api.ts bodies'
// raw-RPC portions exactly.
//
// unlockVaultRpc / lockVaultRpc are deliberately atomic-only: unlike the
// original unlockVault/lockVault (which also drive the vaultUnlocked store,
// refresh folders/connections/identities/protocols/appearance settings, and
// clear stores on lock), these functions perform ONLY their respective RPC
// call. That orchestration remains in stores/api.ts (unlockVault/lockVault)
// until a later task relocates it to actions/.
import { getGateway } from '../backend/context';

export async function unlockVaultRpc(masterPassword: string): Promise<void> {
  const app = getGateway();
  if (!app) return;
  await app.UnlockVault(masterPassword);
}

export async function lockVaultRpc(): Promise<void> {
  const app = getGateway();
  if (!app) return;
  await app.LockVault();
}
