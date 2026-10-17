import { conflictFieldLabel, itemLabel, kindLabel, syncStateLabel } from './syncLabels';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

const pack: Record<string, string> = {
  'sync.kind.connection': 'Connection',
  'sync.kind.other': 'Item',
  'sync.kind.settings': 'Settings: {section}',
  'sync.kind.settingsAny': 'Settings',
  'sync.kind.section.lockout': 'lockout',
  'sync.item.named': '{kind} «{name}»',
  'sync.field.users': 'User',
  'sync.field.passwordId': 'Password',
  'sync.field.host': 'Host',
  'sync.state.idle': 'Up to date',
  'sync.state.waiting': 'Up to date, with {count} change(s) waiting for you',
  'sync.state.running': 'Syncing',
};
// The same contract as $t: a template with {vars}, and the key itself when the packs have nothing.
const t = (key: string, vars?: Record<string, string | number>) =>
  (pack[key] ?? key).replace(/\{(\w+)\}/g, (_, v: string) => String(vars?.[v] ?? ''));

assert(kindLabel('connection', t) === 'Connection', 'a known type is named');
assert(kindLabel('settings.lockout', t) === 'Settings: lockout', 'a settings record names its section');
assert(kindLabel('settings.newer', t) === 'Settings', 'a section this build does not know is still settings');
assert(kindLabel('from-a-newer-build', t) === 'Item', 'an unknown type gets a word, never its raw name');

assert(itemLabel('connection', 'prod-web', t) === 'Connection «prod-web»', 'an item is its kind and its name');
assert(itemLabel('connection', '', t) === 'Connection', 'an item without a name is its kind');

// The conflict from the manual test, as it now reads.
const path = ['users', '[id=u-f57787d1-3c5b-442d-ae7b-29ded98762fb]', 'passAuth', 'passwordId'];
const got = conflictFieldLabel(path, ['', 'deploy', '', ''], t);
assert(got === 'User «deploy» · Password', `field = ${got}`);
assert(!got.includes('u-f577') && !got.includes('['), 'no selector or id reaches the screen');

assert(conflictFieldLabel(['users', '[id=u-1]', 'passAuth', 'passwordId'], undefined, t) === 'User · Password',
  'an element whose name is gone still reads, without its id');
assert(conflictFieldLabel(['host'], [''], t) === 'Host', 'a top-level field');
assert(conflictFieldLabel(['someNewField'], [''], t) === 'someNewField', 'an untranslated field keeps its own name');

console.log('syncLabels tests passed');

assert(syncStateLabel({ state: 'idle', held: 0 }, t) === 'Up to date', 'a settled vault with nothing waiting is up to date');
assert(
  syncStateLabel({ state: 'idle', held: 2 }, t) === 'Up to date, with 2 change(s) waiting for you',
  'a change waiting for the master password is said, not hidden behind "Up to date"',
);
assert(syncStateLabel({ state: 'running', held: 2 }, t) === 'Syncing', 'a running pass says it is running');
