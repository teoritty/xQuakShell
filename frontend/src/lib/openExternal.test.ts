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

assert(openExternal('https://github.com/teoritty/xQuakShell/releases/'), 'an https link is opened');
assert(
  live.opened.length === 1 && live.opened[0] === 'https://github.com/teoritty/xQuakShell/releases/',
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
  links.opened[0] === 'https://github.com/teoritty/xQuakShell/releases/',
  `releases URL = ${links.opened[0]}; the owner is teoritty, not xQuakShell`
);

assert(openNewIssue(), 'Report an Issue opens a bare new-issue form');
assert(
  links.opened[1] === 'https://github.com/teoritty/xQuakShell/issues/new',
  `issue URL = ${links.opened[1]}; a bare call must not append an empty query`
);

// The error dialog prefills the form with the message and stack trace, and those contain
// characters (&, #, newlines) that a hand-built query string mangles.
assert(openNewIssue('boom & crash', 'line1\nline2#end'), 'a prefilled report opens');
const prefilled = new URL(links.opened[2]);
assert(
  prefilled.searchParams.get('title') === 'boom & crash',
  `title round-trips, got ${prefilled.searchParams.get('title')}`
);
assert(
  prefilled.searchParams.get('body') === 'line1\nline2#end',
  `body round-trips through encoding, got ${JSON.stringify(prefilled.searchParams.get('body'))}`
);

// --- no runtime is a no-op, not a crash: unit tests and the log viewer window have none ---
setRuntime(null);
assert(!openExternal('https://example.com'), 'without a runtime the call reports failure');
assert(!openReleasesPage(), 'and the project links report it too rather than throwing');

console.log('openExternal.test passed');
