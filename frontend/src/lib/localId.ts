// Locally generated, UI-only identifiers.
//
// These ids are not secrets, but they must not be *predictable* either: a
// `Date.now()` + `Math.random()` id is guessable from outside the app, and an
// id that another party can guess is an id another party can collide with or
// address. The Web Crypto API is the only source used here on purpose — there
// is deliberately no `Math.random()` fallback, because a silent downgrade to a
// weak generator is exactly the defect this module exists to remove.
//
// The backend remains the canonical ID source wherever one exists; these are
// for rows that only ever live in the browser (see
// connectionDetails/hopIds.ts and connectionDetails/forwardRuleIds.ts, which
// follow the same convention for their draft-row ids).

/** Generates a random, non-guessable id with the given prefix. */
export function newLocalId(prefix: string): string {
  if (typeof crypto !== 'undefined' && typeof crypto.randomUUID === 'function') {
    return `${prefix}${crypto.randomUUID()}`;
  }
  // randomUUID needs a secure context; getRandomValues does not. Wails serves
  // the app from a localhost origin, so the branch above is what normally runs.
  const buf = new Uint8Array(16);
  crypto.getRandomValues(buf);
  return prefix + Array.from(buf, (b) => b.toString(16).padStart(2, '0')).join('');
}
