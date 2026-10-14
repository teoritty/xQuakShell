// The sync half of the Wails bridge (ADR-023), typed structurally like the rest of AppGateway.
// It is its own interface so the gateway file, which every feature touches, does not grow by a
// screenful each time one arrives.
import type { wails } from '../../wailsjs/go/models';

export interface SyncStatus {
  enabled: boolean;
  running: boolean;
  state: 'off' | 'idle' | 'running' | 'failed' | 'stopped';
  message?: string;
  held: number;
  quarantined: number;
  newDevices?: string[];
  serverUrl?: string;
  username?: string;
  fingerprint?: string;
  rekeyPending?: boolean;
  joinPending?: boolean;
}

export interface SyncServerProbe {
  identityKey: string;
  fingerprint: string;
}

export interface EnableSyncRequest {
  serverUrl: string;
  identityKey: string;
  username: string;
  password: string;
  invite: string;
  register: boolean;
}

export interface SyncDevice {
  id: string;
  name: string;
  platform: string;
  createdUnixMilli: number;
  revoked: boolean;
  current: boolean;
}

export interface SyncHeld {
  record: string;
  type: string;
  key: string;
  reason: string;
  pending: boolean;
  atUnixMilli: number;
}

export interface SyncConflict {
  id: string;
  recordType: string;
  itemKey: string;
  path: string[];
  secret: boolean;
  local?: string;
  remote?: string;
  atUnixMilli: number;
}

export interface SyncTrashItem {
  record: string;
  type: string;
  key: string;
  deletedUnixMilli: number;
  purgeUnixMilli: number;
  restorable: boolean;
}

/** A plugin another device recorded that needs the user here (spec 6.4). */
export interface SyncPluginOffer {
  pluginId: string;
  version: string;
  state: 'install' | 'manual' | 'mismatch' | 'removed' | 'update';
  repo?: string;
  tag?: string;
}

export interface SyncPluginInstallRequest {
  pluginId: string;
  secret: boolean;
  auth: boolean;
  tunnel: boolean;
  multiSession: boolean;
  network: boolean;
  exec: boolean;
  acceptDifferent: boolean;
}

/** 'mismatch': the release differs from the recorded build and nothing was installed. */
export interface SyncPluginInstallResult {
  status: 'installed' | 'mismatch';
}

/** A plugin whose data follows the user (spec 6.5). */
export interface SyncPluginData {
  pluginId: string;
  name: string;
  enabled: boolean;
  waiting: number;
  files: number;
}

/** The SyncPluginData event: file names only. */
export interface SyncPluginDataReport {
  pluginId: string;
  conflicts: string[] | null;
  skipped: string[] | null;
}

export interface SyncGateway {
  GetSyncStatus(): Promise<SyncStatus>;
  ProbeSyncServer(serverUrl: string): Promise<SyncServerProbe>;
  EnableSync(req: EnableSyncRequest): Promise<{ joinNeeded: boolean }>;
  JoinSync(choice: string): Promise<string>;
  ReloginSync(password: string): Promise<void>;
  ChangeSyncPassword(oldPassword: string, newPassword: string): Promise<void>;
  ResetSyncPassword(newPassword: string): Promise<void>;
  RevokeSyncDevice(deviceId: string, password: string): Promise<void>;
  DisableSync(): Promise<void>;
  SyncNow(): Promise<void>;
  ListSyncDevices(): Promise<SyncDevice[]>;
  ListSyncHeld(): Promise<SyncHeld[]>;
  ConfirmSyncHeld(record: string, masterPassword: string): Promise<void>;
  RejectSyncHeld(record: string): Promise<void>;
  ListSyncConflicts(): Promise<SyncConflict[]>;
  ResolveSyncConflict(id: string, useOther: boolean): Promise<void>;
  ListSyncTrash(): Promise<SyncTrashItem[]>;
  RestoreSyncTrash(record: string): Promise<void>;
  ShredSyncNow(masterPassword: string): Promise<void>;
  ListSyncPluginOffers(): Promise<SyncPluginOffer[]>;
  PreviewSyncPluginOffer(pluginId: string): Promise<wails.GitHubPluginPreviewResponseDTO>;
  InstallSyncPluginOffer(req: SyncPluginInstallRequest): Promise<SyncPluginInstallResult>;
  KeepSyncPlugin(pluginId: string): Promise<void>;
  ListSyncPluginData(): Promise<SyncPluginData[]>;
  SetSyncPluginData(pluginId: string, enabled: boolean): Promise<void>;
  RestartSyncPlugin(pluginId: string): Promise<void>;
}
