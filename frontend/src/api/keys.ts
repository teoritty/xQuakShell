// Atomic key-manager RPC wrappers. One function per backend call, routed through callBackend for
// uniform error handling, with no store access — orchestration lives in actions/keyActions.ts.
//
// Nothing here ever returns a private key except exportKey, which is the single call the user has
// to re-authenticate for. Keeping that the only exit means a reviewer can check one function
// instead of auditing every listing path for what it might be carrying.
import { callBackend, callBackendVoid } from '../backend/callBackend';
import type { StoredKeyShape } from '../backend/gateway';

export type KeyPolicy = 'vault' | 'passphrase';
export type CachePolicy = 'never' | 'duration' | 'until-lock';

export interface StoredKey {
  id: string;
  comment: string;
  keyType: string;
  bits?: number;
  publicKey?: string;
  fingerprint?: string;
  encrypted: boolean;
  policy?: KeyPolicy;
  cachePolicy?: CachePolicy;
  cacheTtlSeconds?: number;
  allowPlugins: boolean;
  nonExportable: boolean;
  migrationPending: boolean;
  createdAt?: string;
  source?: string;
}

export interface KeyUsage {
  connectionId: string;
  connectionName: string;
  username: string;
  hop?: string;
}

export interface KeyOptions {
  cachePolicy: CachePolicy;
  cacheTtlSeconds: number;
  allowPlugins: boolean;
  nonExportable: boolean;
}

export interface PendingKey {
  id: string;
  comment: string;
  keyType: string;
}

export interface MigrationPlan {
  required: boolean;
  keys: PendingKey[];
}

export interface MigrationReport {
  fromVersion: number;
  toVersion: number;
  converted: string[];
  skipped: string[];
  backupPath: string;
  // Set when the upgrade also gave the vault its first recovery key. A schema this old predates
  // the second credential entirely, so this is the one moment those installations are offered one.
  recoveryKey?: string;
}

export interface DeployResult {
  added: boolean;
  alreadyPresent: boolean;
  path: string;
}

export const defaultKeyOptions: KeyOptions = {
  cachePolicy: 'until-lock',
  cacheTtlSeconds: 900,
  allowPlugins: false,
  nonExportable: false,
};

// The wire carries policy and cache policy as plain strings. Narrowing them here rather than
// asserting keeps an unrecognised value - an older backend, a hand-edited vault - from flowing
// into the UI as a policy nothing renders; it falls back to the behaviour the app had before the
// policy existed, which is the same default the backend applies.
function toStoredKey(raw: StoredKeyShape): StoredKey {
  return {
    ...raw,
    policy: raw.policy === 'passphrase' ? 'passphrase' : 'vault',
    cachePolicy: toCachePolicy(raw.cachePolicy),
  };
}

function toCachePolicy(value: string | undefined): CachePolicy {
  return value === 'never' || value === 'duration' || value === 'until-lock' ? value : 'until-lock';
}

export async function fetchKeys(): Promise<StoredKey[]> {
  return callBackend('Load keys', [] as StoredKey[], async (app) => ((await app.GetKeys()) || []).map(toStoredKey));
}

export async function fetchKeyUsages(id: string): Promise<KeyUsage[]> {
  return callBackend('Load key usage', [] as KeyUsage[], async (app) => (await app.GetKeyUsages(id)) || []);
}

export async function generateKey(
  algorithm: string,
  bits: number,
  comment: string,
  passphrase: string,
  options: KeyOptions,
): Promise<StoredKey | null> {
  return callBackend('Generate key', null as StoredKey | null, async (app) =>
    toStoredKey(await app.GenerateKey(algorithm, bits, comment, passphrase, options)),
  );
}

export async function importKey(
  pemBase64: string,
  passphrase: string,
  comment: string,
  options: KeyOptions,
): Promise<StoredKey | null> {
  return callBackend('Import key', null as StoredKey | null, async (app) =>
    toStoredKey(await app.ImportKey(pemBase64, passphrase, comment, options)),
  );
}

export async function renameKey(id: string, comment: string): Promise<void> {
  return callBackendVoid('Rename key', (app) => app.RenameKey(id, comment));
}

export async function setKeyPolicy(id: string, options: KeyOptions): Promise<void> {
  return callBackendVoid('Update key policy', (app) => app.SetKeyPolicy(id, options));
}

export async function changeKeyPassphrase(id: string, oldPassphrase: string, newPassphrase: string): Promise<void> {
  return callBackendVoid('Change key passphrase', (app) =>
    app.ChangeKeyPassphrase(id, oldPassphrase, newPassphrase),
  );
}

// deleteKey rethrows so the caller can show the list of connections still using the key. Swallowing
// the error would leave the user with a key that silently refuses to go away and no reason given.
export async function deleteKey(id: string): Promise<void> {
  return callBackendVoid('Delete key', (app) => app.DeleteKey(id), { rethrow: true });
}

// exportKey returns base64 of an OpenSSH private key file. It is the only call in this module that
// carries private material, and the only one that takes the master password.
export async function exportKey(
  id: string,
  masterPassword: string,
  passphrase: string,
  exportPassphrase: string,
): Promise<string> {
  return callBackend('Export key', '', (app) => app.ExportKey(id, masterPassword, passphrase, exportPassphrase), {
    rethrow: true,
  });
}

export async function deployKey(sessionId: string, identityId: string): Promise<DeployResult | null> {
  return callBackend('Publish key', null as DeployResult | null, (app) => app.DeployKey(sessionId, identityId), {
    rethrow: true,
  });
}

export async function planKeyMigration(masterPassword: string): Promise<MigrationPlan> {
  return callBackend('Check vault upgrade', { required: false, keys: [] } as MigrationPlan, (app) =>
    app.PlanKeyMigration(masterPassword), { rethrow: true },
  );
}

export async function completeKeyMigration(
  masterPassword: string,
  answers: Record<string, string>,
): Promise<MigrationReport | null> {
  return callBackend('Upgrade vault', null as MigrationReport | null, (app) =>
    app.CompleteKeyMigration(masterPassword, answers), { rethrow: true },
  );
}
