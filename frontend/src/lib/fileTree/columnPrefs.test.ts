// frontend/src/lib/fileTree/columnPrefs.test.ts
import {
  parseColumnPrefs,
  loadPrefs,
  saveColumnPrefs,
  saveHiddenPref,
  NO_COLUMNS,
  type PrefsStore,
} from './columnPrefs';

function assert(cond: boolean, msg: string) {
  if (!cond) throw new Error('FAIL: ' + msg);
}
const KEYS = { columns: 'pane-columns', hidden: 'pane-hidden' };

function store(data: Record<string, string> = {}): PrefsStore & { data: Record<string, string> } {
  return {
    data,
    getItem: (k) => (k in data ? data[k] : null),
    setItem: (k, v) => {
      data[k] = v;
    },
  };
}
// Private-mode Safari and locked-down embedders throw rather than returning null.
const hostileStore: PrefsStore = {
  getItem() {
    throw new Error('SecurityError');
  },
  setItem() {
    throw new Error('SecurityError');
  },
};

// --- parseColumnPrefs survives whatever is in storage ---
assert(parseColumnPrefs(null).permissions === false, 'nothing stored means all columns off');
assert(parseColumnPrefs('').date === false, 'an empty string means all columns off');
assert(parseColumnPrefs('not json').owner === false, 'malformed JSON means all columns off');
assert(parseColumnPrefs('null').owner === false, 'a stored null means all columns off');
assert(parseColumnPrefs('"a string"').owner === false, 'JSON that is not an object means all columns off');
assert(parseColumnPrefs('[1,2]').owner === false, 'an array is not a prefs object');

let p = parseColumnPrefs('{"permissions":true,"owner":false,"date":true}');
assert(p.permissions && !p.owner && p.date, 'stored prefs round-trip');
p = parseColumnPrefs('{"permissions":1,"owner":"yes"}');
assert(p.permissions === true && p.owner === true && p.date === false, 'truthy values coerce to booleans, missing keys to false');
assert(parseColumnPrefs('{}').permissions === false, 'an empty object means all columns off');

// --- the all-off default is never handed out by reference ---
const a = parseColumnPrefs(null);
a.permissions = true;
assert(NO_COLUMNS.permissions === false, 'mutating a parsed result must not corrupt the shared default');

// --- loadPrefs ---
let s = store({ 'pane-columns': '{"owner":true}', 'pane-hidden': '1' });
let loaded = loadPrefs(s, KEYS);
assert(loaded.columns.owner && loaded.showHidden, 'both halves load');
loaded = loadPrefs(store(), KEYS);
assert(!loaded.columns.owner && !loaded.showHidden, 'an empty store loads the defaults');
loaded = loadPrefs(store({ 'pane-hidden': '0' }), KEYS);
assert(!loaded.showHidden, 'only the literal "1" means hidden files are shown');
loaded = loadPrefs(hostileStore, KEYS);
assert(!loaded.columns.owner && !loaded.showHidden, 'a storage that throws opens the pane with defaults');

// --- saving ---
s = store();
saveColumnPrefs(s, KEYS, { permissions: true, owner: false, date: true });
assert(loadPrefs(s, KEYS).columns.permissions === true, 'saved columns are readable again');
saveHiddenPref(s, KEYS, true);
assert(s.data['pane-hidden'] === '1', 'the hidden flag is stored as "1"');
saveHiddenPref(s, KEYS, false);
assert(s.data['pane-hidden'] === '0', 'and as "0" when off');

// --- a storage that refuses writes must not break the pane ---
saveColumnPrefs(hostileStore, KEYS, NO_COLUMNS);
saveHiddenPref(hostileStore, KEYS, true);

console.log('OK fileTree/columnPrefs');
