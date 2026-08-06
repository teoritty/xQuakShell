// The one guard between a plugin tab and a session-only backend call.
//
// activeTabId holds either a sessionId or a surfaceId (ADR-015). Three call sites used to read it
// and hand the result to SendTerminalInput — the audit log's Re-run, the scripts dialog's run, and
// the preset hotkey, which had no check at all. A surface id fails in the backend, and api/terminal
// swallows that failure, so the symptom was a button that did nothing. activeSession is what those
// sites ask now, and null is the answer that stops them.
import { get } from 'svelte/store';
import { activeSession, activeTabId, sessions, type Session } from './appState';

function assert(c: boolean, m: string) {
  if (!c) throw new Error(m);
}

function session(id: string): Session {
  return {
    sessionId: id,
    connectionId: 'c1',
    connectionName: 'host',
    state: 'ready',
  } as Session;
}

sessions.set([session('s1'), session('s2')]);

// The ordinary case: the active tab is a session and resolves to it.
activeTabId.set('s1');
assert(get(activeSession)?.sessionId === 's1', 'a session tab resolves to its session');

activeTabId.set('s2');
assert(get(activeSession)?.sessionId === 's2', 'the store follows the active tab');

// The case this file exists for. A surface id is a legal value of activeTabId and names no session.
activeTabId.set('srf-1');
assert(get(activeSession) === null, 'a plugin surface tab resolves to no session');

// Nothing open at all.
activeTabId.set('');
assert(get(activeSession) === null, 'no active tab is no session');

// A session that closed while it was focused. The id lingers in the store for the moment before
// reconciliation catches up, and it must not resolve to the stale object.
activeTabId.set('s1');
sessions.set([session('s2')]);
assert(get(activeSession) === null, 'a closed session does not linger as the active one');

console.log('activeSession.test.ts: all passed');
