// Sync orchestration: each action calls one or two RPCs and keeps the status store current. Every
// action that can fail returns the message to show next to the form - an empty string is success -
// so a component never has to know how an error is shaped.
import {
  acknowledgeSyncDevicesRpc,
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
  type SyncHeld,
  type SyncServerProbe,
} from '../api/sync';
import { listTeamsRpc } from '../api/team';
import { syncRevision, syncStatus } from '../stores/syncState';
import { teams } from '../stores/teamState';
import { get } from 'svelte/store';
import { refreshFolders } from './folderActions';
import { refreshAllConnections, refreshIdentities } from './connectionActions';
import { applyAppearanceSettings } from './settingsActions';
import { parseSyncLink, sameFingerprint } from '../lib/settings/sync/syncLink';
import { translate } from '../i18n/messages';

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

/**
 * Re-reads the team list every view places items by. With sync off there are no teams; a failed
 * read keeps the last list, since a team does not stop existing because one request failed.
 */
export async function refreshTeams(): Promise<void> {
  if (!get(syncStatus).enabled) {
    teams.set([]);
    return;
  }
  try {
    teams.set(await listTeamsRpc());
  } catch {
    // The Sync page shows the reason when the person opens it.
  }
}

export async function refreshSyncStatus(): Promise<void> {
  try {
    syncStatus.set(await getSyncStatusRpc());
  } catch {
    // The status keeps its last value; the next event or refresh corrects it.
  }
}

const REFRESH_FOLD_MS = 1500;
let queuedRefresh: Promise<void> | null = null;

/**
 * Re-reads everything sync can change that the app keeps on screen - the tree, the keys, the
 * appearance - and tells every open list to reload. Each refresh owns its own error display.
 *
 * Calls that arrive within REFRESH_FOLD_MS are folded into one. Several of the lists re-verify
 * against the server when they reload, and a pass over several vaults reports each of them in turn:
 * reloading once per report sent enough requests at once that the server's per-address rate limit
 * (40 at once, 20 a second) answered "Too many attempts" in the middle of the Sync page. A pass
 * over a handful of vaults finishes inside the window, so it costs one reload.
 */
export function refreshAfterSync(): Promise<void> {
  if (!queuedRefresh) {
    queuedRefresh = new Promise((resolve) => {
      setTimeout(() => {
        queuedRefresh = null;
        void Promise.all([refreshFolders(), refreshAllConnections(), refreshIdentities(), applyAppearanceSettings(), refreshTeams()])
          .finally(() => {
            syncRevision.update((n) => n + 1);
            resolve();
          });
      }, REFRESH_FOLD_MS);
    });
  }
  return queuedRefresh;
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

/** Everything the one-screen sign-in collects. */
export interface ConnectSyncForm {
  /** A bare address or a connect link. */
  address: string;
  username: string;
  password: string;
  /** Present: a new account is created with it. Empty: sign in to an existing one. */
  invite: string;
  /** A fingerprint the person pasted in; takes the place of the link's. */
  expectedFingerprint: string;
  /** The person compared the fingerprint on screen themselves. */
  confirmed: boolean;
  /** When this device and the account both hold data: make this device a copy of the account. */
  replace: boolean;
}

/**
 * The server's identity must be vouched for before anything but a hello is sent: by a fingerprint
 * from the link or pasted in, which must match exactly, or else by the person's own comparison.
 * Returns '' when it is, else the reason to show.
 */
export function identityVouched(form: ConnectSyncForm, probe: SyncServerProbe): string {
  const expected = form.expectedFingerprint.trim() || parseSyncLink(form.address)?.fingerprint || '';
  if (expected) return sameFingerprint(expected, probe.fingerprint) ? '' : translate('security.sync.connect.mismatch');
  return form.confirmed ? '' : translate('security.sync.connect.compareFirst');
}

/**
 * Signs in or registers, and, when both sides hold data, joins - merging unless the person asked
 * for a replace. The vault is backed up before a join either way. probe must be the one taken for
 * this address; a stale probe is refused rather than pinned.
 */
export async function connectSync(form: ConnectSyncForm, probe: SyncServerProbe & { url: string }): Promise<string> {
  const link = parseSyncLink(form.address);
  if (!link || link.url !== probe.url) return translate('sync.connect.badAddress');
  const unvouched = identityVouched(form, probe);
  if (unvouched) return unvouched;
  const invite = form.invite.trim();
  const res = await enableSync({ serverUrl: link.url, identityKey: probe.identityKey, username: form.username.trim(),
    password: form.password, invite, register: invite !== '' });
  if (res.error || !res.joinNeeded) {
    if (!res.error) await refreshAfterSync();
    return res.error;
  }
  return (await joinSync(form.replace ? 'replace' : 'merge')).error;
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
export const acknowledgeSyncDevices = (ids: string[]) => attempt(() => acknowledgeSyncDevicesRpc(ids));
export async function disableSync(): Promise<string> {
  const error = await attempt(() => disableSyncRpc());
  await refreshTeams();
  return error;
}
export const syncNow = () => attempt(() => syncNowRpc());
export const confirmSyncHeld = (held: Pick<SyncHeld, 'vault' | 'record'>, masterPassword: string) =>
  mutate(() => confirmSyncHeldRpc(held.vault, held.record, masterPassword));
export const rejectSyncHeld = (held: Pick<SyncHeld, 'vault' | 'record'>) => mutate(() => rejectSyncHeldRpc(held.vault, held.record));
export const resolveSyncConflict = (id: string, useOther: boolean) => mutate(() => resolveSyncConflictRpc(id, useOther));
export const restoreSyncTrash = (record: string) => mutate(() => restoreSyncTrashRpc(record));

/** Shred now cannot be undone; an empty master password is refused here so it is never sent. */
export async function shredSyncNow(masterPassword: string): Promise<string> {
  if (masterPassword === '') return 'Enter the master password.';
  return attempt(() => shredSyncNowRpc(masterPassword));
}
