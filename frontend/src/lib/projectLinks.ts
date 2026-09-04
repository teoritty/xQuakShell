import { openExternal } from './openExternal';

// Where this project lives, in one place. These were written out at each call site before, and had
// already drifted: ErrorDialog pointed at github.com/xQuakShell/xQuakShell, an owner that does not
// exist, so its "Report an Issue" button would have reached a 404 even once it started opening a
// browser at all.
const REPO_URL = 'https://gitlab.com/teoritty/xQuakShell';
export const RELEASES_URL = `${REPO_URL}/-/releases`;
export const NEW_ISSUE_URL = `${REPO_URL}/-/issues/new`;

// GitLab namespaces a prefilled issue form's fields, where GitHub takes them bare. Sending
// GitHub's ?title=&body= here is not an error the user ever sees - the form just opens empty and
// the report arrives with the message and stack trace missing.
const ISSUE_TITLE_PARAM = 'issue[title]';
const ISSUE_BODY_PARAM = 'issue[description]';

/** Opens the releases page, which is what "Check for Updates" means here - there is no updater. */
export function openReleasesPage(): boolean {
  return openExternal(RELEASES_URL);
}

/**
 * The longest URL that survives being handed to the operating system.
 *
 * Wails' BrowserOpenURL is ShellExecute on Windows, and ShellExecute stops at
 * INTERNET_MAX_URL_LENGTH — 2083 characters — by refusing, with no error anyone can catch and no
 * browser window. That is the whole of the "Open issue on GitLab" bug: the button worked from the
 * About tab, where the URL is bare, and did nothing at all from the error dialog, where a stack
 * trace goes in the query and percent-encoding roughly doubles it. The margin below the true limit
 * is for the browser and GitLab, which have their own ideas about long query strings.
 */
const MAX_EXTERNAL_URL = 1900;

/**
 * Opens a prefilled new-issue form, and reports whether the body survived intact.
 *
 * The query is built with URLSearchParams rather than by hand: the caller passes a raw error
 * message and stack trace, and hand-rolled encodeURIComponent had already been applied
 * inconsistently across the two call sites that needed it.
 *
 * `complete` is false when the body had to be cut to fit, so the caller can put the whole report
 * somewhere the user can still reach it. Truncating silently would be the same bug over again in a
 * quieter form: the report opens, looks finished, and arrives without the trace that mattered.
 */
export function openNewIssue(title?: string, body?: string): { opened: boolean; complete: boolean } {
  const url = new URL(NEW_ISSUE_URL);
  if (title) url.searchParams.set(ISSUE_TITLE_PARAM, title);
  if (!body) return { opened: openExternal(url.toString()), complete: true };

  const fitted = fitBody(url, body);
  url.searchParams.set(ISSUE_BODY_PARAM, fitted);
  return { opened: openExternal(url.toString()), complete: fitted === body };
}

/**
 * The longest prefix of body whose encoded URL still fits.
 *
 * Binary search rather than a character budget, because the ratio between a character and its
 * encoded length is not fixed: a stack trace of ASCII and newlines encodes at about 1.5x, a
 * Cyrillic error message at 9x. Any single guessed ratio is wrong for one of them, and being wrong
 * high is the failure this exists to prevent.
 */
function fitBody(url: URL, body: string): string {
  if (encodedLength(url, body) <= MAX_EXTERNAL_URL) return body;

  let low = 0;
  let high = body.length;
  while (low < high) {
    const mid = Math.ceil((low + high) / 2);
    if (encodedLength(url, body.slice(0, mid) + TRUNCATION_MARKER) <= MAX_EXTERNAL_URL) {
      low = mid;
    } else {
      high = mid - 1;
    }
  }
  return body.slice(0, low) + TRUNCATION_MARKER;
}

const TRUNCATION_MARKER = '\n\n*(truncated — the full report is on your clipboard)*';

function encodedLength(url: URL, body: string): number {
  const probe = new URL(url.toString());
  probe.searchParams.set(ISSUE_BODY_PARAM, body);
  return probe.toString().length;
}
