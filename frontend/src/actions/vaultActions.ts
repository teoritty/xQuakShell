// Vault orchestration layer: composes the atomic vault RPCs
// (createVaultRpc / unlockVaultRpc / lockVaultRpc / vaultExistsRpc, in
// api/vault.ts) with the store-refresh
// warmup / teardown. Moved verbatim from stores/api.ts (unlockVault,
// lockVault) except that the raw app.UnlockVault / app.LockVault calls are
// now routed through the atomic RPC wrappers, and the missing-gateway guard
// (previously implicit in the single `getApp()` check ahead of every store
// mutation) is re-added here explicitly, since unlockVaultRpc/lockVaultRpc
// no longer perform it themselves (each atomic wrapper's own missing-gateway
// behavior is a silent no-op of only the RPC call, not of the surrounding
// store orchestration).
import { getGateway } from '../backend/context';
import {
  unlockVaultRpc,
  lockVaultRpc,
  createVaultRpc,
  vaultExistsRpc,
  acknowledgeRecoveryKeyRpc,
  completeRecoveryResetRpc,
  issueRecoveryKeyRpc,
  changeMasterPasswordRpc,
} from '../api/vault';
import { getPlatform } from '../api/sessions';
import {
  folders,
  connections,
  sessions,
  identities,
  vaultUnlocked,
  vaultExists,
  pendingRecoveryKey,
  recoveryResetRequired,
  platform,
  showError,
} from '../stores/appState';
import { refreshFolders } from './folderActions';
import { refreshAllConnections, refreshIdentities } from './connectionActions';
import { refreshConnectionProtocols } from './protocolActions';
import { applyAppearanceSettings } from './settingsActions';

function handleError(e: unknown, context?: string) {
  const msg = e instanceof Error ? e.message : String(e);
  const message = context ? `${context}: ${msg}` : msg;
  const details = e instanceof Error && e.stack ? e.stack : '';
  showError(message, details);
}

// Store hydration shared by unlockVault and createVault. Both leave the vault
// open and drop the user straight into the app, so both must fill exactly the
// same stores in the same order; keeping it in one place is what stops the two
// entry points from drifting apart.
export async function warmupAfterVaultOpened(): Promise<void> {
  vaultUnlocked.set(true);
  const p = await getPlatform();
  platform.set(p);
  await refreshFolders();
  await refreshAllConnections();
  await refreshIdentities();
  await refreshConnectionProtocols();
  await applyAppearanceSettings();
}

// Opens the vault with whichever credential was typed into the single field.
//
// A recovery unlock deliberately does not warm the stores up. The vault is readable, but the
// password is gone and the credential that got in is one the user was told to keep on paper; the
// reset screen runs before anything else starts.
export async function unlockVault(credential: string): Promise<void> {
  // Mirrors the original stores/api.ts guard: on a missing gateway, do
  // nothing observable (no store mutation, no error toast) and return.
  if (!getGateway()) return;
  const outcome = await unlockVaultRpc(credential);

  if (outcome.method === 'recovery') {
    recoveryResetRequired.set(true);
    return;
  }
  if (outcome.recoveryKey) {
    // This unlock upgraded a vault written before recovery keys existed, so it minted a first one.
    pendingRecoveryKey.set(outcome.recoveryKey);
  }
  await warmupAfterVaultOpened();
}

// Creates the vault on first run. Deliberately without a try/catch: the create
// screen surfaces the failure next to the password field, exactly as the unlock
// screen does.
export async function createVault(masterPassword: string): Promise<void> {
  if (!getGateway()) return;
  const key = await createVaultRpc(masterPassword);
  vaultExists.set(true);
  if (key) pendingRecoveryKey.set(key);
  await warmupAfterVaultOpened();
}

// Sets the master password after a recovery unlock and hands back a fresh key to show.
//
// The stores are warmed here rather than at unlock, so the application only comes up once the
// forgotten credential has actually been replaced.
export async function completeRecoveryReset(newPassword: string): Promise<void> {
  if (!getGateway()) return;
  const key = await completeRecoveryResetRpc(newPassword);
  recoveryResetRequired.set(false);
  if (key) pendingRecoveryKey.set(key);
  await warmupAfterVaultOpened();
}

// Mints a replacement recovery key from settings, revoking the previous one.
export async function regenerateRecoveryKey(masterPassword: string): Promise<void> {
  if (!getGateway()) return;
  const key = await issueRecoveryKeyRpc(masterPassword);
  if (key) pendingRecoveryKey.set(key);
}

// Changes the master password, which also revokes the old recovery key and issues a new one.
export async function changeMasterPassword(current: string, next: string): Promise<void> {
  if (!getGateway()) return;
  const key = await changeMasterPasswordRpc(current, next);
  if (key) pendingRecoveryKey.set(key);
}

// Dismisses the one-time dialog: the backend forgets its copy, then the store forgets the display
// copy. In that order, so a failure to reach the backend leaves the key on screen rather than
// wiping it from the UI while the backend still holds it.
export async function acknowledgeRecoveryKey(): Promise<void> {
  try {
    await acknowledgeRecoveryKeyRpc();
  } catch (e) {
    handleError(e, 'Acknowledge recovery key');
    return;
  }
  pendingRecoveryKey.set(null);
}

// Answers the first-run question for the vault gate. Until this resolves,
// vaultExists stays null and the gate renders neither screen.
export async function initVaultGate(): Promise<void> {
  if (!getGateway()) {
    vaultExists.set(false);
    return;
  }
  vaultExists.set(await vaultExistsRpc());
}

export async function lockVault(): Promise<void> {
  // Mirrors the original stores/api.ts guard: on a missing gateway, do
  // nothing observable (no store mutation, no error toast) and return.
  if (!getGateway()) return;
  try {
    await lockVaultRpc();
  } catch (e) {
    handleError(e, 'Lock vault');
  }
  vaultUnlocked.set(false);
  folders.set([]);
  connections.set([]);
  sessions.set([]);
  identities.set([]);
  // A key still on screen when the vault locks is a key nobody can act on any more, and the backend
  // has dropped its own copy along with everything else.
  pendingRecoveryKey.set(null);
  recoveryResetRequired.set(false);
}
