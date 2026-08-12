// Orchestration over the key-manager RPCs: combines api/keys with the stores the panel renders
// from. Components call these, never the api layer directly.
import { get, writable } from 'svelte/store';
import {
  changeKeyPassphrase,
  deleteKey,
  deployKey,
  exportKey,
  fetchKeyUsages,
  fetchKeys,
  generateKey,
  importKey,
  renameKey,
  setKeyPolicy,
  type DeployResult,
  type KeyOptions,
  type KeyUsage,
  type StoredKey,
} from '../api/keys';

export const storedKeys = writable<StoredKey[]>([]);
export const keysLoading = writable(false);
export const selectedKeyId = writable<string>('');

export async function refreshKeys(): Promise<void> {
  keysLoading.set(true);
  try {
    const keys = await fetchKeys();
    storedKeys.set(sortKeys(keys));
    keepSelectionValid(keys);
  } finally {
    keysLoading.set(false);
  }
}

// Keys sort by label so the list does not reorder under the user when one is renamed or added.
// Map iteration order on the Go side is deliberately random, so without this the list would
// shuffle on every refresh.
function sortKeys(keys: StoredKey[]): StoredKey[] {
  return [...keys].sort((a, b) => a.comment.localeCompare(b.comment) || a.id.localeCompare(b.id));
}

// A selection pointing at a deleted key would leave the details pane rendering nothing with no
// way back, so it falls to the first key instead.
function keepSelectionValid(keys: StoredKey[]): void {
  const current = get(selectedKeyId);
  if (current && keys.some((key) => key.id === current)) return;
  selectedKeyId.set(keys.length > 0 ? keys[0].id : '');
}

export async function createKey(
  algorithm: string,
  bits: number,
  comment: string,
  passphrase: string,
  options: KeyOptions,
): Promise<boolean> {
  const created = await generateKey(algorithm, bits, comment, passphrase, options);
  if (!created) return false;
  await refreshKeys();
  selectedKeyId.set(created.id);
  return true;
}

export async function addKeyFromFile(
  pemBase64: string,
  passphrase: string,
  comment: string,
  options: KeyOptions,
): Promise<boolean> {
  const imported = await importKey(pemBase64, passphrase, comment, options);
  if (!imported) return false;
  await refreshKeys();
  selectedKeyId.set(imported.id);
  return true;
}

export async function saveKeyName(id: string, comment: string): Promise<void> {
  await renameKey(id, comment);
  await refreshKeys();
}

export async function saveKeyPolicy(id: string, options: KeyOptions): Promise<void> {
  await setKeyPolicy(id, options);
  await refreshKeys();
}

export async function updateKeyPassphrase(id: string, oldPassphrase: string, newPassphrase: string): Promise<void> {
  await changeKeyPassphrase(id, oldPassphrase, newPassphrase);
  await refreshKeys();
}

// removeKey rethrows: a refused delete carries the list of connections still using the key, and
// the caller is the only place that can show it.
export async function removeKey(id: string): Promise<void> {
  await deleteKey(id);
  await refreshKeys();
}

export async function loadKeyUsages(id: string): Promise<KeyUsage[]> {
  return fetchKeyUsages(id);
}

export async function saveKeyToDisk(
  id: string,
  masterPassword: string,
  passphrase: string,
  exportPassphrase: string,
): Promise<string> {
  return exportKey(id, masterPassword, passphrase, exportPassphrase);
}

export async function publishKey(sessionId: string, identityId: string): Promise<DeployResult | null> {
  return deployKey(sessionId, identityId);
}
