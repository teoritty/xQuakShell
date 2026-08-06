// Tab-level orchestration (ADR-015): a tab id names either an SSH session or a plugin surface,
// and every action on "the tab" has to route on which.
//
// The regression this file exists for: the close hotkey called closeSession with whatever
// activeTabId held. For a surface that reached the backend as CloseSession("srf-…"), whose
// "session not found" was swallowed — the plugin's tab stayed open, no error was shown, and focus
// moved away from it anyway.
import { get } from 'svelte/store';
import { setGateway } from '../backend/context';
import { createFakeGateway, type FakeGateway } from '../backend/fakeGateway';
import { activeTabId, sessions, lastError } from '../stores/appState';
import { surfaces, type Surface } from '../stores/surfaceState';
import { allTabIds, closeActiveTab, closeTab, focusNextTab, focusPrevTab } from './tabActions';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

function session(id: string) {
  return { sessionId: id, connectionId: 'c-' + id, connectionName: id, state: 'ready', errorMessage: '' } as any;
}

function surface(id: string): Surface {
  return {
    surfaceId: id,
    connectionId: 'c1',
    pluginId: 'p1',
    kind: 'log',
    title: id,
    iconId: '',
    state: 'ready',
    errorMessage: '',
  };
}

function reset(): FakeGateway {
  sessions.set([]);
  surfaces.set([]);
  activeTabId.set('');
  lastError.set(null);
  const fake = createFakeGateway();
  fake.program('CloseSession', undefined);
  fake.program('CloseSurface', undefined);
  setGateway(fake);
  return fake;
}

function methodsCalled(fake: FakeGateway): string[] {
  return fake.calls.map((c) => c.method);
}

async function testClosingASurfaceTabDoesNotTouchSessions() {
  const fake = reset();
  sessions.set([session('s1')]);
  surfaces.set([surface('srf-1')]);
  activeTabId.set('srf-1');

  await closeActiveTab();

  assert(
    methodsCalled(fake).includes('CloseSurface'),
    'closing a surface tab must reach CloseSurface'
  );
  assert(
    !methodsCalled(fake).includes('CloseSession'),
    'a surface id must never be handed to CloseSession'
  );
  assert(get(sessions).length === 1, 'closing a surface must not disturb the sessions');
}

async function testClosingASessionTabIsUnchanged() {
  const fake = reset();
  sessions.set([session('s1'), session('s2')]);
  activeTabId.set('s1');

  await closeActiveTab();

  assert(methodsCalled(fake).includes('CloseSession'), 'closing a session tab reaches CloseSession');
  assert(!methodsCalled(fake).includes('CloseSurface'), 'a session id must not reach CloseSurface');
  assert(get(sessions).length === 1, 'the closed session is removed optimistically');
  assert(get(activeTabId) === 's2', 'focus moves to the remaining tab');
}

async function testFocusAfterClosingTheLastTab() {
  reset();
  sessions.set([session('s1')]);
  activeTabId.set('s1');

  await closeActiveTab();
  assert(get(activeTabId) === '', 'closing the only tab leaves no active tab');
}

// The surface store is emptied by the PluginSurfaceClosed event, which has not arrived yet when
// the hotkey returns. Focus must not land back on the tab that was just closed.
async function testFocusSkipsTheTabJustClosed() {
  reset();
  sessions.set([session('s1')]);
  surfaces.set([surface('srf-1')]);
  activeTabId.set('srf-1');

  await closeActiveTab();
  assert(
    get(activeTabId) === 's1',
    'focus must move off the closed surface even before its event arrives'
  );
}

async function testCloseTabIgnoresAnUnknownId() {
  const fake = reset();
  await closeTab('nothing-here');
  assert(fake.calls.length === 0, 'an id naming no tab is a no-op, not a backend call');
}

function testCycleCoversBothKinds() {
  reset();
  sessions.set([session('s1'), session('s2')]);
  surfaces.set([surface('srf-1')]);

  activeTabId.set('s2');
  focusNextTab();
  assert(get(activeTabId) === 'srf-1', 'cycling forward reaches a plugin tab');

  focusNextTab();
  assert(get(activeTabId) === 's1', 'cycling wraps from the last tab back to the first');

  focusPrevTab();
  assert(get(activeTabId) === 'srf-1', 'cycling back wraps to the last tab');

  activeTabId.set('s1');
  focusNextTab();
  assert(get(activeTabId) === 's2', 'cycling forward moves one tab');
}

function testCycleWithNoTabs() {
  reset();
  focusNextTab();
  assert(get(activeTabId) === '', 'cycling with no tabs leaves the active tab unchanged');
}

function testTabOrderPutsSessionsFirst() {
  reset();
  sessions.set([session('s1')]);
  surfaces.set([surface('srf-1')]);
  assert(
    allTabIds().join(',') === 's1,srf-1',
    'sessions come first, so a window with no plugin tabs cycles exactly as it did before'
  );
}

async function run() {
  await testClosingASurfaceTabDoesNotTouchSessions();
  await testClosingASessionTabIsUnchanged();
  await testFocusAfterClosingTheLastTab();
  await testFocusSkipsTheTabJustClosed();
  await testCloseTabIgnoresAnUnknownId();
  testCycleCoversBothKinds();
  testCycleWithNoTabs();
  testTabOrderPutsSessionsFirst();
  console.log('tabActions.test passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
