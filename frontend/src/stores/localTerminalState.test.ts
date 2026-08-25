// The third tab kind: a local shell.
//
// The tab machinery routes on what an id names, and there are now three answers. Every one of
// these tests exists because getting the routing wrong is silent - a close aimed at the wrong
// subsystem returns "not found", the error is swallowed, and the tab simply stays on screen.
import { get } from 'svelte/store';
import { sessions } from './appState';
import { surfaces, resolveTabIn, resolveTab, tabTitle, tabState, type Surface } from './surfaceState';
import {
  localTerminals,
  upsertLocalTerminal,
  removeLocalTerminal,
  localTerminalIds,
} from './localTerminalState';
import * as localTerminalStore from './localTerminalState';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

function surface(id: string): Surface {
  return {
    surfaceId: id,
    connectionId: 'c1',
    pluginId: 'p1',
    kind: 'log',
    title: id,
    iconId: '',
    state: 'error',
    errorMessage: 'boom',
  };
}

function session(id: string) {
  return { sessionId: id, connectionId: 'c-' + id, connectionName: id, state: 'ready', errorMessage: '' } as any;
}

function reset() {
  sessions.set([]);
  surfaces.set([]);
  localTerminals.set([]);
}

function run(name: string, fn: () => void) {
  reset();
  try {
    fn();
  } catch (e) {
    console.error(`✗ ${name}`);
    throw e;
  }
}

run('upsert adds then replaces, so a repeated event is idempotent', () => {
  upsertLocalTerminal({ id: 'lt-1', title: 'bash' });
  upsertLocalTerminal({ id: 'lt-1', title: 'bash 2' });
  const list = get(localTerminals);
  assert(list.length === 1, `expected one terminal, got ${list.length}`);
  assert(list[0].title === 'bash 2', 'the second upsert must replace, not append');
});

run('remove drops only the named terminal', () => {
  upsertLocalTerminal({ id: 'lt-1', title: 'bash' });
  upsertLocalTerminal({ id: 'lt-2', title: 'zsh' });
  removeLocalTerminal('lt-1');
  assert(localTerminalIds().join(',') === 'lt-2', 'remove hit the wrong terminal');
});

run('resolveTabIn finds a local terminal that is neither a session nor a surface', () => {
  const tab = resolveTabIn([], [], [{ id: 'lt-1', title: 'pwsh' }], 'lt-1');
  assert(tab?.kind === 'local', `kind = ${tab?.kind}, want local`);
});

run('resolveTabIn keeps the three id spaces apart', () => {
  const s = [session('sess-1')];
  const f = [surface('srf-1')];
  const l = [{ id: 'lt-1', title: 'bash' }];

  assert(resolveTabIn(s, f, l, 'sess-1')?.kind === 'session', 'a session id must resolve to a session');
  assert(resolveTabIn(s, f, l, 'srf-1')?.kind === 'surface', 'a surface id must resolve to a surface');
  assert(resolveTabIn(s, f, l, 'lt-1')?.kind === 'local', 'a local id must resolve to a local terminal');
  assert(resolveTabIn(s, f, l, 'lt-nope') === null, 'an unknown id must miss, not match something');
});

run('resolveTab reads the live stores', () => {
  upsertLocalTerminal({ id: 'lt-9', title: 'fish' });
  assert(resolveTab('lt-9')?.kind === 'local', 'resolveTab must see the local terminal store');
});

run('tabTitle shows the shell name', () => {
  const tab = resolveTabIn([], [], [{ id: 'lt-1', title: 'pwsh 2' }], 'lt-1');
  assert(tabTitle(tab) === 'pwsh 2', `title = ${tabTitle(tab)}, want the disambiguated shell name`);
});

run('tabTitle falls back rather than rendering an empty tab', () => {
  const tab = resolveTabIn([], [], [{ id: 'lt-1', title: '' }], 'lt-1');
  assert(tabTitle(tab) === 'shell', `title = ${tabTitle(tab)}, want a fallback`);
});

run('a local terminal is always ready', () => {
  // It has no connecting phase and no remote end that can refuse, so the tab must not carry a
  // status dot suggesting otherwise - and must certainly not inherit a surface's error state.
  const tab = resolveTabIn([], [], [{ id: 'lt-1', title: 'bash' }], 'lt-1');
  assert(tabState(tab) === 'ready', `state = ${tabState(tab)}, want ready`);
});

run('the store offers no way to clear every terminal at once', () => {
  // This asserts an absence, and the absence is the feature. Surfaces export clearLocalTerminals'
  // counterpart and events/subscribe.ts calls it when the vault locks, because a plugin's tabs
  // belong to sessions that no longer exist. A local shell belongs to the user's machine and
  // holds nothing from the vault; a lock that killed a running build would be the worst surprise
  // this feature could produce. If a clear function ever appears here, something will call it.
  const clearing = Object.keys(localTerminalStore).filter((k) => /^clear/i.test(k));
  assert(clearing.length === 0, `the store exports ${clearing.join(', ')}; a vault lock must not kill a shell`);
});

console.log('localTerminalState.test passed');
