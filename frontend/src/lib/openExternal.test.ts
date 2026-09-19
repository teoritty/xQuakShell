// openExternal is the app's only door out to the operating system, so both halves of it are
// asserted here: that a legitimate link is handed to the Wails runtime rather than to window.open
// (the bug in issue #41), and that anything which is not http/https is refused before it reaches
// the shell.
import { setRuntime } from '../backend/context';
import type { RuntimeGateway } from '../backend/gateway';
import { openExternal } from './openExternal';
import { openNewIssue, openReleasesPage } from './projectLinks';

function assert(cond: boolean, msg: string): void {
  if (!cond) throw new Error('FAIL: ' + msg);
}

function fakeRuntime(): { runtime: RuntimeGateway; opened: string[] } {
  const opened: string[] = [];
  return {
    opened,
    runtime: {
      EventsOn: () => undefined,
      BrowserOpenURL: (url: string) => opened.push(url),
    },
  };
}

// --- the fix itself: the URL reaches the runtime, so the OS opens a real browser ---
const live = fakeRuntime();
setRuntime(live.runtime);

assert(openExternal('https://github.com/teoritty/xQuakShell/releases'), 'an https link is opened');
assert(
  live.opened.length === 1 && live.opened[0] === 'https://github.com/teoritty/xQuakShell/releases',
  `runtime saw ${JSON.stringify(live.opened)}; the URL must reach BrowserOpenURL unchanged`
);

assert(openExternal('http://example.com/x?a=1&b=2'), 'an http link is opened');
assert(
  live.opened[1] === 'http://example.com/x?a=1&b=2',
  'the query string survives: the issue reporter carries its title and body that way'
);

// --- everything else is refused before the shell ever sees it ---
for (const hostile of [
  'file:///etc/passwd',
  'file://C:/Windows/System32/calc.exe',
  'smb://attacker/share',
  'javascript:alert(1)',
  'data:text/html,<script>alert(1)</script>',
  'vscode://x',
  'not a url at all',
  '',
]) {
  const before = live.opened.length;
  assert(!openExternal(hostile), `refused: ${hostile || '(empty)'}`);
  assert(
    live.opened.length === before,
    `${hostile || '(empty)'} reached BrowserOpenURL; only http and https may leave the app`
  );
}

// --- the project's own links, which are what issue #41 was actually about ---
const links = fakeRuntime();
setRuntime(links.runtime);

assert(openReleasesPage(), 'Check for Updates opens the releases page');
assert(
  links.opened[0] === 'https://github.com/teoritty/xQuakShell/releases',
  `releases URL = ${links.opened[0]}; releases ship on GitHub`
);

const bare = openNewIssue();
assert(bare.opened, 'Report an Issue opens a bare new-issue form');
assert(bare.complete, 'a bare call has nothing to truncate');
assert(
  links.opened[1] === 'https://github.com/teoritty/xQuakShell/issues/new',
  `issue URL = ${links.opened[1]}; a bare call must not append an empty query`
);

// The error dialog prefills the form with the message and stack trace, and those contain
// characters (&, #, newlines) that a hand-built query string mangles. GitHub reads the fields
// as bare title and body; any other name - GitLab's issue[...] among them - opens the form empty,
// which is a silent failure: the user files a report and the diagnostics are simply absent.
const short = openNewIssue('boom & crash', 'line1\nline2#end');
assert(short.opened, 'a prefilled report opens');
assert(short.complete, 'a short report is sent whole');
const prefilled = new URL(links.opened[2]);
assert(
  prefilled.searchParams.get('title') === 'boom & crash',
  `title round-trips under GitHub's field name, got ${prefilled.searchParams.get('title')}`
);
assert(
  prefilled.searchParams.get('body') === 'line1\nline2#end',
  `body round-trips through encoding, got ${JSON.stringify(prefilled.searchParams.get('body'))}`
);
assert(
  prefilled.searchParams.get('issue[title]') === null && prefilled.searchParams.get('issue[description]') === null,
  "GitLab's issue[...] names must not be sent: GitHub ignores them and the form opens empty"
);

// --- the length limit, which is what actually broke this button ---
//
// Wails' BrowserOpenURL is ShellExecute on Windows, and ShellExecute refuses a URL past
// INTERNET_MAX_URL_LENGTH without an error anyone can catch. A stack trace in the query is always
// past it, so "Open issue" did nothing from the error dialog while working from the About
// tab, where the URL is bare. A URL that is short enough to be handed over is the property; the
// report saying so is what lets the caller offer the clipboard instead.
const trace = 'at frame ' + 'x'.repeat(40) + '\n';
const huge = openNewIssue('crash', trace.repeat(400));
assert(huge.opened, 'an over-long report still opens the form rather than doing nothing');
assert(!huge.complete, 'and it says the body did not fit, so the caller can offer the whole text');
const cut = new URL(links.opened[3]);
assert(
  links.opened[3].length <= 1900,
  `URL length = ${links.opened[3].length}; past the shell's limit the call is dropped in silence`
);
const cutBody = cut.searchParams.get('body') ?? '';
assert(cutBody.startsWith('at frame'), 'the head of the report is what survives, not the tail');
assert(
  cutBody.includes('truncated'),
  'the truncation is visible in the issue itself, so nobody files half a trace believing it whole'
);

// A body of multi-byte characters encodes to roughly nine times its length, against about 1.5x for
// an ASCII trace. A fixed character budget would be right for one and badly wrong for the other,
// and wrong-high is the direction that puts the button back to doing nothing.
const cyrillic = openNewIssue('сбой', 'Ошибка соединения. '.repeat(200));
assert(cyrillic.opened, 'a Cyrillic report opens');
assert(!cyrillic.complete, 'and reports being cut');
assert(
  links.opened[4].length <= 1900,
  `Cyrillic URL length = ${links.opened[4].length}; encoded length is what counts, not character count`
);

// --- no runtime is a no-op, not a crash: unit tests and the log viewer window have none ---
setRuntime(null);
assert(!openExternal('https://example.com'), 'without a runtime the call reports failure');
assert(!openReleasesPage(), 'and the project links report it too rather than throwing');

console.log('openExternal.test passed');
