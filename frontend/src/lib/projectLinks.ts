import { openExternal } from './openExternal';

// Where this project lives, in one place. These were written out at each call site before, and had
// already drifted: ErrorDialog pointed at github.com/xQuakShell/xQuakShell, an owner that does not
// exist, so its "Report an Issue" button would have reached a 404 even once it started opening a
// browser at all.
const REPO_URL = 'https://github.com/teoritty/xQuakShell';
export const RELEASES_URL = `${REPO_URL}/releases/`;
export const NEW_ISSUE_URL = `${REPO_URL}/issues/new`;

/** Opens the releases page, which is what "Check for Updates" means here - there is no updater. */
export function openReleasesPage(): boolean {
  return openExternal(RELEASES_URL);
}

/**
 * Opens a prefilled new-issue form.
 *
 * The query is built with URLSearchParams rather than by hand: the caller passes a raw error
 * message and stack trace, and hand-rolled encodeURIComponent had already been applied
 * inconsistently across the two call sites that needed it.
 */
export function openNewIssue(title?: string, body?: string): boolean {
  const url = new URL(NEW_ISSUE_URL);
  if (title) url.searchParams.set('title', title);
  if (body) url.searchParams.set('body', body);
  return openExternal(url.toString());
}
