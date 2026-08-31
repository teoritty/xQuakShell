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

// Which credential opened the vault. A recovery unlock has to land on the reset
// screen rather than the connection list, so the caller needs to be told.
export type UnlockMethod = 'password' | 'recovery';

export interface UnlockOutcome {
  method: UnlockMethod;
  // Set only when this unlock upgraded a vault written before recovery keys
  // existed and therefore minted a first key to show. Empty every other time.
  recoveryKey: string;
}

// The backendless (browser dev) run has no vault at all, so a missing gateway
// answers "password" rather than throwing: the create screen is the safe
// default there, and actions/vaultActions owns the real decision.
export async function unlockVaultRpc(credential: string): Promise<UnlockOutcome> {
  const app = getGateway();
  if (!app) return { method: 'password', recoveryKey: '' };
  const result = await app.UnlockVault(credential);
  return {
    method: result?.method === 'recovery' ? 'recovery' : 'password',
    recoveryKey: result?.recoveryKey ?? '',
  };
}

export async function createVaultRpc(masterPassword: string): Promise<string> {
  const app = getGateway();
  if (!app) return '';
  const result = await app.CreateVault(masterPassword);
  return result?.key ?? '';
}

// Returns false when the gateway is missing, matching the "do nothing
// observable" contract of the wrappers above. The create screen is the safe
// default for a backendless (browser dev) run; actions/vaultActions owns the
// real decision.
export async function vaultExistsRpc(): Promise<boolean> {
  const app = getGateway();
  if (!app) return false;
  return await app.VaultExists();
}

export async function lockVaultRpc(): Promise<void> {
  const app = getGateway();
  if (!app) return;
  await app.LockVault();
}

export async function issueRecoveryKeyRpc(masterPassword: string): Promise<string> {
  const app = getGateway();
  if (!app) return '';
  const result = await app.IssueRecoveryKey(masterPassword);
  return result?.key ?? '';
}

export async function changeMasterPasswordRpc(current: string, next: string): Promise<string> {
  const app = getGateway();
  if (!app) return '';
  const result = await app.ChangeMasterPassword(current, next);
  return result?.key ?? '';
}

export async function completeRecoveryResetRpc(newPassword: string): Promise<string> {
  const app = getGateway();
  if (!app) return '';
  const result = await app.CompleteRecoveryReset(newPassword);
  return result?.key ?? '';
}

// Tells the backend to forget the key it is holding for the dialog. After this
// resolves the key cannot be shown or saved again by anything.
export async function acknowledgeRecoveryKeyRpc(): Promise<void> {
  const app = getGateway();
  if (!app) return;
  await app.AcknowledgeRecoveryKey();
}

// Opens the platform's save dialog and writes the held key. Resolves false when
// the user cancelled, which is not a failure: the key is still on screen.
export async function saveRecoveryKeyFileRpc(): Promise<boolean> {
  const app = getGateway();
  if (!app) return false;
  return await app.SaveRecoveryKeyFile();
}

export async function hasRecoveryKeyRpc(): Promise<boolean> {
  const app = getGateway();
  if (!app) return false;
  return await app.HasRecoveryKey();
}
