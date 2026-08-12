// Atomic credentials RPC wrappers. Each function is a thin wrapper around a single backend RPC
// call, routed through callBackend for uniform error handling, with no store access.
//
// Keys are deliberately absent: they are reached through api/keys.ts, which is the one door into
// the key manager's pool. The listing and file-import wrappers that used to live here were a
// second way in, and having two is how the manager ended up looking like a viewer for ~/.ssh.
import { callBackend, callBackendVoid } from '../backend/callBackend';
import type { Connection } from '../stores/appState';

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
