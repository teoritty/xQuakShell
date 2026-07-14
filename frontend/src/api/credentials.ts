// Atomic credentials/identity RPC wrappers. Each function is a thin wrapper
// around a single backend RPC call, routed through callBackend for uniform
// error handling. No store access here — orchestration (refreshIdentities in
// stores/api.ts) combines fetchIdentities with the `identities` store.
import { callBackend, callBackendVoid } from '../backend/callBackend';
import type { Connection, SSHIdentityMeta } from '../stores/appState';

export interface PuTTYSessionPreview {
  name: string;
  hostName: string;
  port: number;
  userName: string;
}

export async function importPassword(password: string, label: string): Promise<string> {
  return callBackend('Import password', '', (app) => app.ImportPassword(password, label));
}

export async function deletePassword(id: string): Promise<void> {
  return callBackendVoid('Delete password', (app) => app.DeletePassword(id));
}

export async function fetchIdentities(): Promise<SSHIdentityMeta[]> {
  return callBackend('Refresh identities', [] as SSHIdentityMeta[], (app) => app.GetIdentities());
}

export async function importIdentity(pemBase64: string, comment: string): Promise<string> {
  return callBackend('Import identity', '', (app) => app.ImportIdentity(pemBase64, comment));
}

export async function importPuTTYPPK(ppkBase64: string, passphrase: string): Promise<string> {
  return callBackend('Import PPK', '', (app) => app.ImportPuTTYPPK(ppkBase64, passphrase));
}

export async function importPuTTYRegPreview(regContent: string): Promise<PuTTYSessionPreview[]> {
  return callBackend('Parse PuTTY REG', [] as PuTTYSessionPreview[], async (app) => (await app.ImportPuTTYReg(regContent)) || []);
}

export async function importPuTTYRegAsConnections(regContent: string, folderId: string): Promise<Connection[]> {
  return callBackend('Import PuTTY sessions', [] as Connection[], async (app) => {
    const result = (await app.ImportPuTTYRegAsConnections(regContent, folderId)) || [];
    return result as Connection[];
  });
}
