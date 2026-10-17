// Reading what a person pastes into the server field: a bare address, or the connect link xqsyncd
// prints, https://host:8443/#fp=04512-99031-…, which carries the fingerprint they would otherwise
// have to compare group by group.
//
// The fingerprint in a link is checked, not trusted: the client probes the server, computes the
// fingerprint of the identity key it was offered, and continues only on an exact match. That is the
// comparison the person would make by reading twelve groups aloud, without the typo, so a link
// makes sign-in one step without making it weaker - a link is exactly as strong as the channel it
// came over, as a fingerprint read out on that channel is.

/** Twelve groups of five digits. */
const FINGERPRINT_DIGITS = 60;

export interface SyncLink {
  /** The server's base URL, without the fragment. */
  url: string;
  /** The fingerprint the link carries, normalised; '' when it carries none. */
  fingerprint: string;
}

/** The digits of a fingerprint as groups of five, or '' when it is not one. */
export function normalizeFingerprint(text: string): string {
  const digits = text.replace(/[\s\-–—]/g, '');
  if (!/^\d+$/.test(digits) || digits.length !== FINGERPRINT_DIGITS) return '';
  return digits.match(/\d{5}/g)!.join(' ');
}

/** Whether two fingerprints are the same one. Two that are not fingerprints never match. */
export function sameFingerprint(a: string, b: string): boolean {
  const na = normalizeFingerprint(a);
  return na !== '' && na === normalizeFingerprint(b);
}

/**
 * Parses a server address or connect link. A host typed without a scheme is taken as https, the
 * only scheme the server speaks; anything else, plain http included, is not a sync server address.
 */
export function parseSyncLink(input: string): SyncLink | null {
  const text = input.trim();
  if (!text) return null;
  let u: URL;
  try {
    u = new URL(/^[a-z][a-z0-9+.-]*:\/\//i.test(text) ? text : `https://${text}`);
  } catch {
    return null;
  }
  if (u.protocol !== 'https:' || !u.hostname || u.username || u.password) return null;
  const fp = new URLSearchParams(u.hash.replace(/^#/, '')).get('fp') ?? '';
  return { url: `https://${u.host}${u.pathname.replace(/\/+$/, '')}`, fingerprint: normalizeFingerprint(fp) };
}
