// Atomic remote-filesystem / SFTP operations. Each function is a thin
// wrapper around a single backend RPC call, routed through callBackend /
// callBackendVoid for uniform error handling. No store access here beyond
// what callBackend does internally (showError on failure).
import { callBackend, callBackendVoid } from '../backend/callBackend';
import type { RemoteNode } from '../stores/appState';

function isCancelError(msg: string): boolean {
  return msg.toLowerCase().includes('cancel');
}

export async function listPath(sessionId: string, path: string): Promise<RemoteNode[]> {
  return callBackend('List remote path', [], (app) => app.ListPath(sessionId, path));
}

export async function uploadFile(sessionId: string, localPath: string, remotePath: string): Promise<void> {
  return callBackendVoid('Upload file', (app) => app.Upload(sessionId, localPath, remotePath), {
    silence: isCancelError,
  });
}

export async function downloadFile(sessionId: string, remotePath: string, localPath: string): Promise<void> {
  return callBackendVoid('Download file', (app) => app.Download(sessionId, remotePath, localPath), {
    silence: isCancelError,
  });
}

export function cancelTransfer(transferId: string): void {
  void callBackendVoid('Cancel transfer', (app) => app.CancelTransfer(transferId));
}

export async function removePath(sessionId: string, path: string): Promise<void> {
  return callBackendVoid('Remove remote path', (app) => app.RemovePath(sessionId, path));
}

export async function mkdirPath(sessionId: string, parentPath: string, name: string): Promise<void> {
  return callBackendVoid('Create remote directory', (app) => app.MkdirPath(sessionId, parentPath, name));
}

export async function createFilePath(sessionId: string, parentPath: string, name: string): Promise<void> {
  return callBackendVoid('Create remote file', (app) => app.CreateFilePath(sessionId, parentPath, name));
}

export async function copyLocalPath(srcPath: string, destDir: string): Promise<void> {
  return callBackendVoid('Copy local path', (app) => app.CopyLocalPath(srcPath, destDir));
}

export async function renamePath(sessionId: string, oldPath: string, newPath: string): Promise<void> {
  return callBackendVoid('Rename remote path', (app) => app.RenamePath(sessionId, oldPath, newPath));
}

export type ApplyTarget = 'files' | 'dirs' | 'both';

export async function chmodPath(sessionId: string, path: string, mode: number): Promise<void> {
  return callBackendVoid('Change permissions', (app) => app.Chmod(sessionId, path, mode));
}

export async function chownPath(sessionId: string, path: string, uid: number, gid: number): Promise<void> {
  return callBackendVoid('Change owner', (app) => app.Chown(sessionId, path, uid, gid));
}

export async function chmodPathRecursive(
  sessionId: string,
  path: string,
  mode: number,
  applyTo: ApplyTarget,
): Promise<void> {
  return callBackendVoid('Change permissions', (app) => app.ChmodRecursive(sessionId, path, mode, applyTo));
}

export async function chownPathRecursive(
  sessionId: string,
  path: string,
  uid: number,
  gid: number,
  applyTo: ApplyTarget,
): Promise<void> {
  return callBackendVoid('Change owner', (app) => app.ChownRecursive(sessionId, path, uid, gid, applyTo));
}
