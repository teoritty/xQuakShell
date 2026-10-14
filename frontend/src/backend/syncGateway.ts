// The sync half of the Wails bridge (ADR-023), typed structurally like the rest of AppGateway.
// It is its own interface so the gateway file, which every feature touches, does not grow by a
// screenful each time one arrives.
import type { wails } from '../../wailsjs/go/models';

export interface SyncStatus {
  enabled: boolean;
  running: boolean;
  state: 'off' | 'idle' | 'running' | 'failed' | 'stopped';
  message?: string;
  /** What a stopped account needs: sign in again, let the removed device go, or look into tampering. */
  stopReason?: 'signIn' | 'revoked' | 'integrity';
  held: number;
  quarantined: number;
  /** Devices that joined after this one, announced until acknowledged. */
  newDevices?: SyncDevice[];
  serverUrl?: string;
  username?: string;
  fingerprint?: string;
  /** This account's own fingerprint, the one a team admin compares before inviting it. */
  accountFingerprint?: string;
  rekeyPending?: boolean;
  joinPending?: boolean;
  /** Changes the reporting round wrote into the vault; above zero, open views re-read it. */
  applied?: number;
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
  /** What the person called the item; absent for a settings section. */
  name?: string;
  reason: string;
  pending: boolean;
  atUnixMilli: number;
}

export interface SyncConflict {
  id: string;
  recordType: string;
  itemKey: string;
  itemName?: string;
  path: string[];
  /** One entry per path segment: the element's name for an "[id=…]" selector, "" otherwise. */
  pathNames: string[];
  secret: boolean;
  local?: string;
  remote?: string;
  atUnixMilli: number;
}

export interface SyncTrashItem {
  record: string;
  type: string;
  key: string;
  name?: string;
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
  AcknowledgeSyncDevices(deviceIds: string[]): Promise<void>;
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
