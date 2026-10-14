// Sync orchestration: each action calls one or two RPCs and keeps the status store current. Every
// action that can fail returns the message to show next to the form - an empty string is success -
// so a component never has to know how an error is shaped.
import {
  changeSyncPasswordRpc,
  confirmSyncHeldRpc,
  disableSyncRpc,
  enableSyncRpc,
  getSyncStatusRpc,
  joinSyncRpc,
  probeSyncServerRpc,
  rejectSyncHeldRpc,
  reloginSyncRpc,
  resetSyncPasswordRpc,
  resolveSyncConflictRpc,
  restoreSyncTrashRpc,
  revokeSyncDeviceRpc,
  shredSyncNowRpc,
  syncNowRpc,
  type EnableSyncRequest,
  type SyncServerProbe,
} from '../api/sync';
import { syncRevision, syncStatus } from '../stores/syncState';
import { refreshFolders } from './folderActions';
import { refreshAllConnections, refreshIdentities } from './connectionActions';
import { applyAppearanceSettings } from './settingsActions';

export function errorMessage(e: unknown): string {
  return e instanceof Error ? e.message : String(e);
}

/** Runs fn and returns '' or the error's message, refreshing the status either way. */
async function attempt(fn: () => Promise<unknown>): Promise<string> {
  let msg = '';
  try {
    await fn();
  } catch (e) {
    msg = errorMessage(e);
  }
  await refreshSyncStatus();
  return msg;
}

/** attempt for an action that writes into the vault: the views re-read it afterwards. */
async function mutate(fn: () => Promise<unknown>): Promise<string> {
  const msg = await attempt(fn);
  await refreshAfterSync();
  return msg;
}

export async function refreshSyncStatus(): Promise<void> {
  try {
    syncStatus.set(await getSyncStatusRpc());
  } catch {
    // The status keeps its last value; the next event or refresh corrects it.
  }
}

/**
 * Re-reads everything sync can change that the app keeps on screen - the tree, the keys, the
 * appearance - and tells every open list to reload. Each refresh owns its own error display.
 */
export async function refreshAfterSync(): Promise<void> {
  await Promise.all([refreshFolders(), refreshAllConnections(), refreshIdentities(), applyAppearanceSettings()]);
  syncRevision.update((n) => n + 1);
}

export async function probeSyncServer(url: string): Promise<{ probe?: SyncServerProbe; error?: string }> {
  try {
    const probe = await probeSyncServerRpc(url.trim());
    return probe ? { probe } : { error: 'Sync is unavailable.' };
  } catch (e) {
    return { error: errorMessage(e) };
  }
}

export async function enableSync(req: EnableSyncRequest): Promise<{ joinNeeded: boolean; error: string }> {
  let joinNeeded = false;
  const error = await attempt(async () => {
    joinNeeded = (await enableSyncRpc(req)).joinNeeded;
  });
  return { joinNeeded, error };
}

export async function joinSync(choice: 'merge' | 'replace'): Promise<{ backup: string; error: string }> {
  let backup = '';
  const error = await mutate(async () => {
    backup = await joinSyncRpc(choice);
  });
  return { backup, error };
}

export const reloginSync = (password: string) => attempt(() => reloginSyncRpc(password));
export const changeSyncPassword = (oldPw: string, newPw: string) => attempt(() => changeSyncPasswordRpc(oldPw, newPw));
export const resetSyncPassword = (newPw: string) => attempt(() => resetSyncPasswordRpc(newPw));
export const revokeSyncDevice = (id: string, password: string) => attempt(() => revokeSyncDeviceRpc(id, password));
export const disableSync = () => attempt(() => disableSyncRpc());
export const syncNow = () => attempt(() => syncNowRpc());
export const confirmSyncHeld = (record: string, masterPassword: string) =>
  mutate(() => confirmSyncHeldRpc(record, masterPassword));
export const rejectSyncHeld = (record: string) => mutate(() => rejectSyncHeldRpc(record));
export const resolveSyncConflict = (id: string, useOther: boolean) => mutate(() => resolveSyncConflictRpc(id, useOther));
export const restoreSyncTrash = (record: string) => mutate(() => restoreSyncTrashRpc(record));

/** Shred now cannot be undone; an empty master password is refused here so it is never sent. */
export async function shredSyncNow(masterPassword: string): Promise<string> {
  if (masterPassword === '') return 'Enter the master password.';
  return attempt(() => shredSyncNowRpc(masterPassword));
}
