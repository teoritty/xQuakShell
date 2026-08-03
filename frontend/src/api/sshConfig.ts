// Atomic ssh_config import RPC wrappers. Each function is a thin wrapper
// around a single backend call, routed through callBackend for uniform error
// handling. No store access here — the dialog owns orchestration.
//
// Note what does not cross this boundary: the frontend sends a file path and a
// list of host aliases, and never handles config contents or key material. The
// backend re-parses the file on import and derives key paths itself.
import { callBackend } from '../backend/callBackend';
import type { Connection } from '../stores/appState';

/** Kinds of non-fatal parse finding. Mirrors domain.SSHConfigNoticeKind. */
export type SSHConfigNoticeKind =
  | 'matchBlockSkipped'
  | 'proxyCommandUnsupported'
  | 'includeUnreadable'
  | 'identityFileMissing'
  | 'jumpHostUnresolved'
  | 'limitReached';

export interface SSHConfigNotice {
  kind: SSHConfigNoticeKind | string;
  target: string;
}

export interface SSHConfigHost {
  alias: string;
  hostName: string;
  port: number;
  user: string;
  keyCount: number;
  jumpAliases: string[];
  duplicate: boolean;
}

export interface SSHConfigPreview {
  path: string;
  hosts: SSHConfigHost[];
  keyFileCount: number;
  notices: SSHConfigNotice[];
}

export interface SSHConfigImportResult {
  connections: Connection[];
  importedKeys: number;
  failedKeys: number;
  skippedAliases: string[];
}

const emptyPreview: SSHConfigPreview = { path: '', hosts: [], keyFileCount: 0, notices: [] };

const emptyResult: SSHConfigImportResult = {
  connections: [],
  importedKeys: 0,
  failedKeys: 0,
  skippedAliases: []
};

/** Returns the detected ~/.ssh/config path, or '' when there is none. */
export async function fetchSSHConfigDefaultPath(): Promise<string> {
  return callBackend('Detect SSH config', '', (app) => app.GetSSHConfigDefaultPath());
}

/** Parses a config file without writing anything to the vault. */
export async function previewSSHConfig(path: string): Promise<SSHConfigPreview> {
  return callBackend('Read SSH config', emptyPreview, async (app) => {
    const result = await app.PreviewSSHConfig(path);
    return normalizePreview(result as unknown as Partial<SSHConfigPreview>);
  });
}

/** Creates vault connections for the selected hosts. */
export async function importSSHConfig(
  path: string,
  aliases: string[],
  folderId: string,
  importKeys: boolean
): Promise<SSHConfigImportResult> {
  return callBackend('Import SSH config', emptyResult, async (app) => {
    const result = await app.ImportSSHConfig(path, aliases, folderId, importKeys);
    return normalizeResult(result as unknown as Partial<SSHConfigImportResult>);
  });
}

// The Go side already sends [] rather than null for optional lists, but a
// dropped bridge call resolves to undefined; normalising here keeps every
// consumer free of null checks.
function normalizePreview(raw: Partial<SSHConfigPreview> | undefined): SSHConfigPreview {
  if (!raw) return emptyPreview;
  return {
    path: raw.path ?? '',
    hosts: (raw.hosts ?? []).map((h) => ({ ...h, jumpAliases: h.jumpAliases ?? [] })),
    keyFileCount: raw.keyFileCount ?? 0,
    notices: raw.notices ?? []
  };
}

function normalizeResult(raw: Partial<SSHConfigImportResult> | undefined): SSHConfigImportResult {
  if (!raw) return emptyResult;
  return {
    connections: raw.connections ?? [],
    importedKeys: raw.importedKeys ?? 0,
    failedKeys: raw.failedKeys ?? 0,
    skippedAliases: raw.skippedAliases ?? []
  };
}
