// State sync RPC wrappers (ADR-023), one per backend method. Errors are rethrown, not shown here:
// the sync forms show them inline next to the field the person has to change, which a global
// error toast would separate from the cause.
import { callBackend } from '../backend/callBackend';

import type {
  EnableSyncRequest,
  SyncConflict,
  SyncDevice,
  SyncHeld,
  SyncServerProbe,
  SyncStatus,
} from '../backend/syncGateway';

export type { EnableSyncRequest, SyncConflict, SyncDevice, SyncHeld, SyncServerProbe, SyncStatus };

const inline = { rethrow: true, silence: () => true };

const offStatus: SyncStatus = { enabled: false, running: false, state: 'off', held: 0, quarantined: 0 };

export function getSyncStatusRpc(): Promise<SyncStatus> {
  return callBackend('Sync status', offStatus, (app) => app.GetSyncStatus(), inline);
}

export function probeSyncServerRpc(serverUrl: string): Promise<SyncServerProbe | null> {
  return callBackend<SyncServerProbe | null>('Sync server', null, (app) => app.ProbeSyncServer(serverUrl), inline);
}

export function enableSyncRpc(req: EnableSyncRequest): Promise<{ joinNeeded: boolean }> {
  return callBackend('Enable sync', { joinNeeded: false }, (app) => app.EnableSync(req), inline);
}

export function joinSyncRpc(choice: 'merge' | 'replace'): Promise<string> {
  return callBackend('Join sync account', '', (app) => app.JoinSync(choice), inline);
}

export function reloginSyncRpc(password: string): Promise<void> {
  return callBackend('Sign in to sync', undefined, (app) => app.ReloginSync(password), inline);
}

export function changeSyncPasswordRpc(oldPassword: string, newPassword: string): Promise<void> {
  return callBackend('Change sync password', undefined, (app) => app.ChangeSyncPassword(oldPassword, newPassword), inline);
}

export function resetSyncPasswordRpc(newPassword: string): Promise<void> {
  return callBackend('Reset sync password', undefined, (app) => app.ResetSyncPassword(newPassword), inline);
}

export function revokeSyncDeviceRpc(deviceId: string, password: string): Promise<void> {
  return callBackend('Remove device', undefined, (app) => app.RevokeSyncDevice(deviceId, password), inline);
}

export function disableSyncRpc(): Promise<void> {
  return callBackend('Disable sync', undefined, (app) => app.DisableSync(), inline);
}

export function syncNowRpc(): Promise<void> {
  return callBackend('Sync now', undefined, (app) => app.SyncNow(), inline);
}

export function listSyncDevicesRpc(): Promise<SyncDevice[]> {
  return callBackend<SyncDevice[]>('Sync devices', [], (app) => app.ListSyncDevices(), inline);
}

export function listSyncHeldRpc(): Promise<SyncHeld[]> {
  return callBackend<SyncHeld[]>('Held changes', [], (app) => app.ListSyncHeld(), inline);
}

export function confirmSyncHeldRpc(record: string, masterPassword: string): Promise<void> {
  return callBackend('Accept change', undefined, (app) => app.ConfirmSyncHeld(record, masterPassword), inline);
}

export function rejectSyncHeldRpc(record: string): Promise<void> {
  return callBackend('Reject change', undefined, (app) => app.RejectSyncHeld(record), inline);
}

export function listSyncConflictsRpc(): Promise<SyncConflict[]> {
  return callBackend<SyncConflict[]>('Sync conflicts', [], (app) => app.ListSyncConflicts(), inline);
}

export function resolveSyncConflictRpc(id: string, useOther: boolean): Promise<void> {
  return callBackend('Resolve conflict', undefined, (app) => app.ResolveSyncConflict(id, useOther), inline);
}
