import type { AuditEntry } from '../api/settings';

/**
 * Presentation rules for one audit row, kept out of the component so they can be tested and so the
 * one place that formats a timestamp knows which language it is formatting for.
 */

/** True for an entry recording that the host refused something. */
export function isDenied(entry: AuditEntry): boolean {
  return entry.input.includes('result=denied');
}

/**
 * The timestamp as the user's language writes it.
 *
 * The locale is passed in rather than left to the platform default: the interface language is a
 * setting, and a Russian interface printing American date order is the kind of mismatch that makes
 * a log hard to read at a glance.
 *
 * An unparseable timestamp is shown verbatim — this is a record, and rendering "Invalid Date" over
 * it loses what was actually written. The check is on the parsed value rather than in a try/catch,
 * because `new Date("not a date")` does not throw: it produces an invalid Date whose
 * `toLocaleString` cheerfully returns the string "Invalid Date", so the catch this replaced could
 * never fire.
 */
export function formatTimestamp(ts: string, locale: string): string {
  const parsed = new Date(ts);
  if (Number.isNaN(parsed.getTime())) return ts;
  try {
    return parsed.toLocaleString(locale);
  } catch {
    // An unrecognised locale tag is the one thing here that does throw.
    return parsed.toLocaleString();
  }
}

/** The connection column: name, host, both, or nothing, depending on what the entry recorded. */
export function formatConnection(entry: AuditEntry): string {
  if (entry.connectionName && entry.host) return `${entry.connectionName} @ ${entry.host}`;
  return entry.connectionName || entry.host || '';
}
