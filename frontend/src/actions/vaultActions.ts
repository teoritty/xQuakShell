// Vault orchestration layer: composes the atomic vault RPCs
// (unlockVaultRpc / lockVaultRpc, in api/vault.ts) with the store-refresh
// warmup / teardown. Moved verbatim from stores/api.ts (unlockVault,
// lockVault) except that the raw app.UnlockVault / app.LockVault calls are
// now routed through the atomic RPC wrappers, and the missing-gateway guard
// (previously implicit in the single `getApp()` check ahead of every store
// mutation) is re-added here explicitly, since unlockVaultRpc/lockVaultRpc
// no longer perform it themselves (each atomic wrapper's own missing-gateway
// behavior is a silent no-op of only the RPC call, not of the surrounding
// store orchestration).
import { getGateway } from '../backend/context';
import { unlockVaultRpc, lockVaultRpc } from '../api/vault';
import { getPlatform } from '../api/sessions';
import { folders, connections, sessions, identities, vaultUnlocked, platform, showError } from '../stores/appState';
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

export async function unlockVault(masterPassword: string): Promise<void> {
  // Mirrors the original stores/api.ts guard: on a missing gateway, do
  // nothing observable (no store mutation, no error toast) and return.
  if (!getGateway()) return;
  await unlockVaultRpc(masterPassword);
  vaultUnlocked.set(true);
  const p = await getPlatform();
  platform.set(p);
  await refreshFolders();
  await refreshAllConnections();
  await refreshIdentities();
  await refreshConnectionProtocols();
  await applyAppearanceSettings();
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
}
