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
/** Resolves a message key. Components pass `$t`; tests pass whatever they need. */
export type ImportLabels = (key: string, vars?: Record<string, string | number>) => string;

export function importButtonLabel(count: number, label: ImportLabels): string {
  if (count === 0) return label('import.selectHosts');
  return label('import.importCount', { count });
}

/**
 * Turns a backend notice into a sentence.
 *
 * The backend deliberately sends only a kind and a short target, never an
 * error message, so the phrasing lives here where it can be read and changed
 * like any other UI copy.
 */
const NOTICE_KEYS: Record<string, string> = {
  matchBlockSkipped: 'import.notice.matchBlockSkipped',
  proxyCommandUnsupported: 'import.notice.proxyCommandUnsupported',
  includeUnreadable: 'import.notice.includeUnreadable',
  identityFileMissing: 'import.notice.identityFileMissing',
  jumpHostUnresolved: 'import.notice.jumpHostUnresolved',
  limitReached: 'import.notice.limitReached',
};

export function describeNotice(notice: SSHConfigNotice, label: ImportLabels): string {
  // The target rides in as a variable rather than being appended, so a translation can put it
  // where its own grammar wants it - or leave it out of a sentence that reads better without.
  const target = notice.target ? ` (${notice.target})` : '';
  return label(NOTICE_KEYS[notice.kind] ?? 'import.notice.unknown', { target });
}

/** Sentence summarising a finished import. */
export function describeResult(
  result: {
    connections: unknown[];
    importedKeys: number;
    failedKeys: number;
    skippedAliases: string[];
  },
  label: ImportLabels,
): string {
  // Each clause is its own key with its own plural forms rather than a noun glued to a verb:
  // "2 connections imported" is one sentence in English and a differently-inflected one in a
  // language that agrees the participle with the number.
  const parts = [label('import.result.connections', { count: result.connections.length })];
  if (result.importedKeys > 0) {
    parts.push(label('import.result.keysAdded', { count: result.importedKeys }));
  }
  if (result.failedKeys > 0) {
    parts.push(label('import.result.keysFailed', { count: result.failedKeys }));
  }
  if (result.skippedAliases.length > 0) {
    parts.push(label('import.result.skipped', { count: result.skippedAliases.length }));
  }
  return parts.join(', ') + '.';
}
