// Pure selection and phrasing helpers for the ssh_config import dialog. Kept
// free of Svelte and of the backend so the rules that decide what gets
// imported — and the sentences shown to the user — can be tested directly.
import type { SSHConfigHost, SSHConfigNotice } from '../../api/sshConfig';

/** State of the header's select-all control. */
export type MasterState = 'none' | 'some' | 'all';

/**
 * Aliases selected by default: everything that is not already in the vault.
 *
 * Duplicates start unselected so that re-running an import after adding two
 * hosts offers those two, rather than forty copies of what is already there.
 */
export function defaultSelection(hosts: SSHConfigHost[]): Set<string> {
  return new Set(hosts.filter((h) => !h.duplicate).map((h) => h.alias));
}

export function toggleAlias(selected: Set<string>, alias: string): Set<string> {
  const next = new Set(selected);
  if (!next.delete(alias)) next.add(alias);
  return next;
}

export function selectAll(hosts: SSHConfigHost[]): Set<string> {
  return new Set(hosts.map((h) => h.alias));
}

export function masterState(hosts: SSHConfigHost[], selected: Set<string>): MasterState {
  const count = hosts.filter((h) => selected.has(h.alias)).length;
  if (count === 0) return 'none';
  return count === hosts.length ? 'all' : 'some';
}

/** Number of distinct key files the current selection would read. */
export function selectedKeyCount(hosts: SSHConfigHost[], selected: Set<string>): number {
  return hosts
    .filter((h) => selected.has(h.alias))
    .reduce((total, h) => total + h.keyCount, 0);
}

export function countDuplicates(hosts: SSHConfigHost[]): number {
  return hosts.filter((h) => h.duplicate).length;
}

/** Label for one host row, e.g. "deploy@web.example.com:2222". */
export function describeHost(host: SSHConfigHost): string {
  const target = host.port && host.port !== 22 ? `${host.hostName}:${host.port}` : host.hostName;
  return host.user ? `${host.user}@${target}` : target;
}

/** Label for the confirm button; states the exact count being acted on. */
export function importButtonLabel(count: number): string {
  if (count === 0) return 'Select hosts to import';
  return count === 1 ? 'Import 1 connection' : `Import ${count} connections`;
}

/**
 * Turns a backend notice into a sentence.
 *
 * The backend deliberately sends only a kind and a short target, never an
 * error message, so the phrasing lives here where it can be read and changed
 * like any other UI copy.
 */
export function describeNotice(notice: SSHConfigNotice): string {
  const target = notice.target ? ` (${notice.target})` : '';
  switch (notice.kind) {
    case 'matchBlockSkipped':
      return 'A Match block was skipped: its conditions can only be evaluated when connecting.';
    case 'proxyCommandUnsupported':
      return `ProxyCommand is not supported and was ignored${target}. Set up a jump host manually if needed.`;
    case 'includeUnreadable':
      return `An included file could not be read${target}.`;
    case 'identityFileMissing':
      return `A referenced key file was not found${target}. Connections using it are imported without it.`;
    case 'jumpHostUnresolved':
      return `A ProxyJump entry could not be resolved${target}. Its jump chain may be incomplete.`;
    case 'limitReached':
      return `The config is larger than the importer reads in one pass${target}. Some entries may be missing.`;
    default:
      return `The config contains something the importer did not handle${target}.`;
  }
}

/** Sentence summarising a finished import. */
export function describeResult(result: {
  connections: unknown[];
  importedKeys: number;
  failedKeys: number;
  skippedAliases: string[];
}): string {
  const parts = [plural(result.connections.length, 'connection', 'connections') + ' imported'];
  if (result.importedKeys > 0) {
    parts.push(plural(result.importedKeys, 'key', 'keys') + ' added');
  }
  if (result.failedKeys > 0) {
    parts.push(plural(result.failedKeys, 'key', 'keys') + ' could not be read');
  }
  if (result.skippedAliases.length > 0) {
    parts.push(`${result.skippedAliases.length} no longer in the config`);
  }
  return parts.join(', ') + '.';
}

function plural(n: number, one: string, many: string): string {
  return `${n} ${n === 1 ? one : many}`;
}
