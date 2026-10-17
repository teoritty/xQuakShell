// A contact code hands a person's fingerprint to whoever adds them to a team, so nobody reads twelve
// groups aloud: xqs-member:alice#fp=91796-59138-…, copied from the Teams page and sent over any
// channel that is not this sync server - a chat, a mail, a ticket.
//
// The fingerprint in a code is checked, not trusted, exactly like the one in a server connect link:
// the lookup replays the account's log from the genesis key the server returns, and the backend
// invites or verifies only on an exact match with that key. A server answering for the username
// with a key of its own therefore fails the check instead of being compared by a tired eye. A code
// is as strong as the channel it came over, which is what a fingerprint read out on that channel is.
//
// The username only says whom to look up. It is not checked against anything, and it needs no
// check: a code naming the wrong account carries that account's fingerprint, not this one's.
import { normalizeFingerprint } from './syncLink';

const PREFIX = 'xqs-member:';
const FRAGMENT = '#fp=';

export interface InviteInput {
  username: string;
  /** The fingerprint the code carries, normalised; '' for a plain username. */
  fingerprint: string;
}

/** This account's code to hand out, or '' while its username or fingerprint is not known. */
export function formatContactCode(username: string, fingerprint: string): string {
  const fp = normalizeFingerprint(fingerprint);
  if (!username || !fp) return '';
  return `${PREFIX}${encodeURIComponent(username)}${FRAGMENT}${fp.replace(/ /g, '-')}`;
}

function parseCode(text: string): InviteInput | null {
  const at = text.lastIndexOf(FRAGMENT);
  if (at < 0) return null;
  const fingerprint = normalizeFingerprint(text.slice(at + FRAGMENT.length));
  let username: string;
  try {
    username = decodeURIComponent(text.slice(PREFIX.length, at));
  } catch {
    return null;
  }
  return username && fingerprint ? { username, fingerprint } : null;
}

/**
 * Reads the invite field: a contact code, or a plain username to compare by hand. A damaged code is
 * null rather than a username, because looking it up as one would skip the check it was sent for.
 */
export function parseInviteInput(input: string): InviteInput | null {
  const text = input.trim();
  if (!text) return null;
  if (text.toLowerCase().startsWith(PREFIX)) return parseCode(text);
  return { username: text, fingerprint: '' };
}

/** The fingerprint in a pasted contact code or bare fingerprint, normalised; '' when there is none. */
export function codeFingerprint(input: string): string {
  const text = input.trim();
  if (text.toLowerCase().startsWith(PREFIX)) return parseCode(text)?.fingerprint ?? '';
  return normalizeFingerprint(text);
}
