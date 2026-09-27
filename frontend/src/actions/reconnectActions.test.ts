import assert from 'node:assert/strict';
import { get } from 'svelte/store';
import type { Terminal } from '@xterm/xterm';
import type { FitAddon } from '@xterm/addon-fit';
import { setGateway } from '../backend/context';
import { createFakeGateway, type FakeGateway } from '../backend/fakeGateway';
import { sessions, activeTabId, lastError, type Session } from '../stores/appState';
import { reconnectCountdowns } from '../stores/reconnectState';
import { disposeTerminal, getPooledTerminal, setPooledTerminal } from '../lib/terminalPool';
import {
  startAutoReconnect,
  reconnectNow,
  stopAutoReconnect,
  refreshAutoReconnectSetting,
  type ReconnectClock,
} from './reconnectActions';

// A clock the test advances by hand, so a 60-second backoff costs nothing to walk through.
function fakeClock() {
  let now = 1_000_000;
  let seq = 0;
  const timers = new Map<number, { at: number; fn: () => void }>();
  const clock: ReconnectClock = {
    now: () => now,
    setTimeout: (fn, ms) => {
      timers.set(++seq, { at: now + ms, fn });
      return seq;
    },
    clearTimeout: (handle) => {
      timers.delete(handle as number);
    },
  };
  return {
    clock,
    /** Delays of the timers still pending, relative to now. */
    pending: () => [...timers.values()].map((t) => t.at - now),
    async advance(ms: number) {
      now += ms;
      for (const [id, t] of [...timers]) {
        if (t.at <= now) {
          timers.delete(id);
          t.fn();
        }
      }
      await settle();
    },
    get now() {
      return now;
    },
  };
}

// Every step of a reconnect is a promise chain over the fake gateway; a macrotask boundary lets all
// of it finish before the test looks.
async function settle() {
  for (let i = 0; i < 3; i++) await new Promise((r) => setTimeout(r, 0));
}

interface Harness {
  fake: FakeGateway;
  time: ReturnType<typeof fakeClock>;
  stop: () => void;
  settings: { autoReconnect: boolean; preserveTerminalContext: boolean };
}

function harness(): Harness {
  sessions.set([]);
  activeTabId.set('');
  lastError.set(null);
  reconnectCountdowns.set({});

  const settings = { autoReconnect: true, preserveTerminalContext: true };
  const fake = createFakeGateway();
  let opened = 0;
  fake.program('GetSettings', () => ({ ...settings }));
  fake.program('OpenSession', () => `new-${++opened}`);
  fake.program('CloseSession', undefined);
  setGateway(fake);

  const time = fakeClock();
  const stop = startAutoReconnect(time.clock);
  return { fake, time, stop, settings };
}

function session(id: string, state: Session['state'], extra: Partial<Session> = {}): Session {
  return { sessionId: id, connectionId: 'c1', connectionName: 'prod', protocol: 'ssh', state, errorMessage: '', ...extra };
}

/** Applies a backend state event to one session, the way events/subscribe.ts merges it. */
async function emit(id: string, patch: Partial<Session>) {
  sessions.update((list) => list.map((s) => (s.sessionId === id ? { ...s, ...patch } : s)));
  await settle();
}

const ids = () => get(sessions).map((s) => s.sessionId);
const opens = (fake: FakeGateway) => fake.calls.filter((c) => c.method === 'OpenSession').length;
const closedIds = (fake: FakeGateway) => fake.calls.filter((c) => c.method === 'CloseSession').map((c) => c.args[0]);

async function run() {
  // A lost connection counts down 5 s, then replaces the session in place and keeps the focus.
  {
    const h = harness();
    sessions.set([session('a', 'ready'), session('s1', 'ready'), session('b', 'ready')]);
    activeTabId.set('s1');
    await settle();

    await emit('s1', { state: 'error', connectionLost: true, errorMessage: 'Connection lost' });
    assert.deepEqual(get(reconnectCountdowns)['s1'], { attempt: 1, deadline: h.time.now + 5000 }, 'a loss starts attempt 1 at 5 s');

    await h.time.advance(4999);
    assert.equal(opens(h.fake), 0, 'nothing is opened before the countdown ends');

    await h.time.advance(1);
    assert.equal(opens(h.fake), 1, 'the expired countdown opens exactly one new session');
    assert.equal(h.fake.calls.find((c) => c.method === 'OpenSession')?.args[0], 'c1', 'for the same connection');
    assert.deepEqual(ids(), ['a', 'new-1', 'b'], 'the new session takes the old one\'s place in the tab order');
    assert.equal(get(activeTabId), 'new-1', 'and the focus the old one had');
    assert.deepEqual(closedIds(h.fake), ['s1'], 'the lost session is closed on the backend');
    assert.deepEqual(get(reconnectCountdowns), {}, 'no countdown is left behind for the replaced session');
    h.stop();
  }

  // Failed attempts back off 15, 30, 60, 60; success resets the chain to 5 s.
  {
    const h = harness();
    sessions.set([session('s1', 'ready')]);
    await settle();
    await emit('s1', { state: 'error', connectionLost: true });
    await h.time.advance(5000);

    const seen: number[] = [];
    for (let n = 1; n <= 4; n++) {
      // A failed attempt is a failed connect: the backend never flags it as a lost connection.
      await emit(`new-${n}`, { state: 'error', connectionLost: false, errorMessage: 'Connection failed' });
      seen.push(h.time.pending()[0]);
      await h.time.advance(h.time.pending()[0]);
    }
    assert.deepEqual(seen, [15000, 30000, 60000, 60000], 'each failed attempt waits longer, capped at 60 s');
    assert.equal(opens(h.fake), 5, 'one open per expired countdown');

    await emit('new-5', { state: 'ready' });
    await emit('new-5', { state: 'error', connectionLost: true });
    assert.deepEqual(h.time.pending(), [5000], 'after a success the next loss starts again from 5 s');
    h.stop();
  }

  // What does NOT start a countdown.
  {
    const h = harness();
    sessions.set([session('exited', 'ready'), session('first', 'connecting')]);
    await settle();
    await emit('exited', { state: 'error', connectionLost: false, errorMessage: 'Remote shell exited' });
    await emit('first', { state: 'error', errorMessage: 'Authentication failed' });
    assert.deepEqual(get(reconnectCountdowns), {}, 'neither a shell exit nor a failed first connect is retried automatically');
    assert.deepEqual(h.time.pending(), [], 'and no timer is armed for them');
    h.stop();
  }

  // The setting turns the whole thing off.
  {
    const h = harness();
    h.settings.autoReconnect = false;
    sessions.set([session('s1', 'ready')]);
    await settle();
    await emit('s1', { state: 'error', connectionLost: true });
    assert.deepEqual(get(reconnectCountdowns), {}, 'auto-reconnect off means no countdown after a loss');
    h.stop();
  }

  // Turning it off in the dialog stops countdowns already running.
  {
    const h = harness();
    sessions.set([session('s1', 'ready')]);
    await settle();
    await emit('s1', { state: 'error', connectionLost: true });
    h.settings.autoReconnect = false;
    await refreshAutoReconnectSetting();
    assert.deepEqual(get(reconnectCountdowns), {}, 'a saved opt-out clears the running countdown');
    await h.time.advance(60_000);
    assert.equal(opens(h.fake), 0, 'and its timer never fires');
    h.stop();
  }

  // Stop retrying: the countdown goes, the tab stays.
  {
    const h = harness();
    sessions.set([session('s1', 'ready')]);
    await settle();
    await emit('s1', { state: 'error', connectionLost: true });
    stopAutoReconnect('s1');
    await h.time.advance(60_000);
    assert.equal(opens(h.fake), 0, 'a stopped countdown never reconnects');
    assert.deepEqual(ids(), ['s1'], 'stopping leaves the failed tab where it is');
    h.stop();
  }

  // Closing the tab during a countdown takes the countdown with it.
  {
    const h = harness();
    sessions.set([session('s1', 'ready')]);
    await settle();
    await emit('s1', { state: 'error', connectionLost: true });
    sessions.set([]);
    await settle();
    assert.deepEqual(get(reconnectCountdowns), {}, 'the countdown goes with the tab');
    assert.deepEqual(h.time.pending(), [], 'and so does its timer');
    await h.time.advance(60_000);
    assert.equal(opens(h.fake), 0, 'a closed tab is never reopened by a leftover timer');
    h.stop();
  }

  // The button during a countdown fires now and keeps the chain; without one it is a one-off.
  {
    const h = harness();
    sessions.set([session('s1', 'ready'), session('manual', 'connecting')]);
    await settle();
    await emit('s1', { state: 'error', connectionLost: true });
    reconnectNow('s1');
    await settle();
    assert.equal(opens(h.fake), 1, 'clicking during the countdown reconnects immediately');
    assert.deepEqual(h.time.pending(), [], 'the countdown it pre-empted is gone');
    await emit('new-1', { state: 'error' });
    assert.deepEqual(h.time.pending(), [15000], 'the clicked attempt still counts: its failure waits 15 s');

    await emit('manual', { state: 'error', errorMessage: 'Authentication failed' });
    reconnectNow('manual');
    await settle();
    await emit('new-2', { state: 'error', errorMessage: 'Authentication failed' });
    assert.equal(get(reconnectCountdowns)['new-2'], undefined, 'a manual retry of a failed first connect starts no chain');
    h.stop();
  }

  // Two clicks in quick succession open one session, not two.
  {
    const h = harness();
    sessions.set([session('s1', 'ready')]);
    await settle();
    await emit('s1', { state: 'error' });
    reconnectNow('s1');
    reconnectNow('s1');
    await settle();
    assert.equal(opens(h.fake), 1, 'a second click while the first is opening is ignored');
    h.stop();
  }

  // Opening fails outright: reported, and not retried on a timer.
  {
    const h = harness();
    h.fake.program('OpenSession', () => {
      throw new Error('connection not found');
    });
    sessions.set([session('s1', 'ready')]);
    await settle();
    await emit('s1', { state: 'error', connectionLost: true });
    await h.time.advance(5000);
    assert.equal(get(lastError)?.message, 'Reconnect session: connection not found', 'the failure is shown to the user');
    assert.deepEqual(h.time.pending(), [], 'a failure no network change will fix does not loop');
    assert.deepEqual(ids(), ['s1'], 'the failed tab stays so the user can act on it');
    h.stop();
  }

  // A session that recovers on its own - a plugin coming back after a process restart, or one with
  // its own reconnect - must not be replaced by the countdown it started.
  {
    const h = harness();
    sessions.set([session('s1', 'ready')]);
    await settle();
    await emit('s1', { state: 'error', connectionLost: true });
    await emit('s1', { state: 'connecting', connectionLost: false, errorMessage: 'Recovering from plugin crash' });
    assert.deepEqual(get(reconnectCountdowns), {}, 'leaving the error cancels its countdown');
    await emit('s1', { state: 'ready' });
    await h.time.advance(60_000);
    assert.equal(opens(h.fake), 0, 'a recovered session is never replaced');
    h.stop();
  }

  // Once opening has failed, the chain is over for good: a later failure of the same session is a
  // plain failure again, not the next step of a chain that ended.
  {
    const h = harness();
    sessions.set([session('s1', 'ready')]);
    await settle();
    await emit('s1', { state: 'error', connectionLost: true });
    await h.time.advance(5000);
    await emit('new-1', { state: 'error' });
    h.fake.program('OpenSession', () => {
      throw new Error('connection not found');
    });
    await h.time.advance(15000);
    await emit('new-1', { state: 'connecting' });
    await emit('new-1', { state: 'error' });
    assert.deepEqual(h.time.pending(), [], 'a chain ended by a failed open does not restart on the next failure');
    h.stop();
  }

  // Context preservation follows the setting.
  for (const preserve of [true, false]) {
    const h = harness();
    h.settings.preserveTerminalContext = preserve;
    const term = { write: () => {}, dispose: () => {} } as unknown as Terminal;
    setPooledTerminal('s1', { term, fitAddon: {} as FitAddon, host: { parentNode: null } as unknown as HTMLDivElement, baseFontSize: 14 });
    sessions.set([session('s1', 'ready')]);
    await settle();
    await emit('s1', { state: 'error', connectionLost: true });
    await h.time.advance(5000);
    assert.equal(
      getPooledTerminal('new-1')?.term === term,
      preserve,
      preserve ? 'with preservation on, the new session inherits the terminal' : 'with preservation off, the new session starts empty',
    );
    // The pool is module state: a terminal left under 'new-1' would make the next iteration's
    // handover refuse for the wrong reason and pass without testing anything.
    disposeTerminal('new-1');
    disposeTerminal('s1');
    h.stop();
  }

  console.log('reconnectActions.test passed');
}

run().catch((e) => {
  console.error(e);
  process.exit(1);
});
