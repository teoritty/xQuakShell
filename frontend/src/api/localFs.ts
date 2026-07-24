// Atomic local-filesystem operations. Each function is a thin wrapper
// around a single backend RPC call, routed through callBackend /
// callBackendVoid for uniform error handling, except where the original
// stores/api.ts body did something callBackend's uniform model can't
// replicate byte-for-byte (documented inline below).
import { callBackend, callBackendVoid } from '../backend/callBackend';
import { getGateway } from '../backend/context';
import { showError } from '../stores/appState';

export interface LocalNode {
  name: string;
  path: string;
  isDir: boolean;
  size: number;
  modTime?: string;
  mode?: string;
  owner?: string;
}

export async function removeLocalPath(localPath: string): Promise<void> {
  return callBackendVoid('Remove local path', (app) => app.RemoveLocalPath(localPath));
}

export async function mkdirLocalPath(dirPath: string): Promise<void> {
  return callBackendVoid('Create local directory', (app) => app.MkdirLocalPath(dirPath));
}

export async function renameLocalPath(oldPath: string, newPath: string): Promise<void> {
  return callBackendVoid('Rename local path', (app) => app.RenameLocalPath(oldPath, newPath));
}

export async function createLocalFile(localPath: string): Promise<void> {
  return callBackendVoid('Create local file', (app) => app.CreateLocalFile(localPath));
}

export async function selectLocalFile(): Promise<string> {
  return callBackend('Select local file', '', (app) => app.SelectLocalFile());
}

export async function selectLocalDirectory(): Promise<string> {
  return callBackend('Select local directory', '', (app) => app.SelectLocalDirectory());
}

export async function listLocalPath(
  dirPath: string,
  includeHidden = false,
  opts?: { rethrow?: boolean; silence?: (msg: string) => boolean },
): Promise<LocalNode[]> {
  return callBackend('List local path', [], (app) => app.ListLocalPath(dirPath, includeHidden), opts);
}

// getPortableDataRoot / getUserHomeDir / getTempDir intentionally do NOT go
// through callBackend's showError path: the original stores/api.ts bodies
// catch with a bare `catch { return '' }` and never call handleError, so
// errors here must stay silent (never surface via lastError). We reimplement
// directly rather than passing `silence: () => true` to callBackend, to keep
// the "these three are silent by design" contract obvious at the call site.
export async function getPortableDataRoot(): Promise<string> {
  const app = getGateway();
  if (!app) return '';
  try {
    // Capability probe: older/alternate backends may not expose
    // GetPortableDataRoot at all, in which case we fall back to the user's
    // home directory instead of failing.
    if (typeof app.GetPortableDataRoot === 'function') {
      return await app.GetPortableDataRoot();
    }
    return await app.GetUserHomeDir();
  } catch {
    return '';
  }
}

export async function getUserHomeDir(): Promise<string> {
  const app = getGateway();
  if (!app) return '';
  try {
    return await app.GetUserHomeDir();
  } catch {
    return '';
  }
}

export async function getTempDir(): Promise<string> {
  const app = getGateway();
  if (!app) return '';
  try {
    return await app.GetTempDir();
  } catch {
    return '';
  }
}

export async function openFileWithSystem(localPath: string, editorPath?: string): Promise<void> {
  return callBackendVoid('Open file', (app) => app.OpenFileWithSystem(localPath, editorPath ?? ''));
}

// Fire-and-forget: matches original stores/api.ts behavior where
// app.StartFileWatch was called without await inside a synchronous
// function. A synchronous throw is caught and reported via showError, but
// since the call is never awaited, an async rejection from the backend
// never surfaces via showError/lastError. The trailing .catch(() => {})
// mirrors cancelTransfer in remoteFs.ts: it only prevents an unhandled
// promise rejection (which would otherwise crash Node/tsx and print an
// unhandled-rejection warning in the browser) — it does not add any
// observable error reporting that wasn't there before.
export function startFileWatch(localPath: string): void {
  const app = getGateway();
  if (!app) return;
  try {
    void app.StartFileWatch(localPath).catch(() => {});
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e);
    const details = e instanceof Error && e.stack ? e.stack : '';
    showError(`Start file watch: ${msg}`, details);
  }
}
