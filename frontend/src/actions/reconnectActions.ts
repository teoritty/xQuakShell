// Automatic reconnect: watches sessions, runs the countdown after a lost connection, and replaces
// the session when it expires. The rules for when to retry live in lib/reconnect/reconnectPolicy.ts;
// this file owns only the timers and the bookkeeping around them.
//
// It lives on the frontend because a reconnect here is close + open (see reconnectSession), which
// already works the same for SSH and for every protocol plugin. The backend's only part is saying
// which failures are lost connections (ConnectionSession.connectionLost).
import { get } from 'svelte/store';
import { sessions, type Session, type SessionState } from '../stores/appState';
import { reconnectCountdowns } from '../stores/reconnectState';
import { getSettings } from './settingsActions';
import { reconnectSession } from './sessionActions';
import { reconnectDecision, reconnectDelaySeconds } from '../lib/reconnect/reconnectPolicy';

/** Time and timers, injectable so tests can drive a countdown without waiting for it. */
export interface ReconnectClock {
  now(): number;
  setTimeout(fn: () => void, ms: number): unknown;
  clearTimeout(handle: unknown): void;
}

const systemClock: ReconnectClock = {
  now: () => Date.now(),
  setTimeout: (fn, ms) => setTimeout(fn, ms),
  clearTimeout: (handle) => clearTimeout(handle as ReturnType<typeof setTimeout>),
};

let clock: ReconnectClock = systemClock;
let unsubscribe: (() => void) | null = null;

const timers = new Map<string, unknown>();
/** Sessions that are themselves automatic attempts, with the number of attempts made so far. */
const attemptsMade = new Map<string, number>();
const lastStates = new Map<string, SessionState>();
/** Sessions whose replacement is being opened right now, so a click cannot open a second one. */
const inFlight = new Set<string>();

/**
 * Starts watching sessions. Idempotent: a second call replaces the first watcher, and everything
 * pending is dropped, so a remount of the app shell never leaves two timers per failure.
 */
export function startAutoReconnect(withClock: ReconnectClock = systemClock): () => void {
  stopWatching();
  clock = withClock;
  unsubscribe = sessions.subscribe(observeSessions);
  return stopWatching;
}

function stopWatching(): void {
  unsubscribe?.();
  unsubscribe = null;
  for (const id of [...lastStates.keys()]) forget(id);
  inFlight.clear();
}

function observeSessions(list: Session[]): void {
  const present = new Set<string>();
  for (const session of list) {
    const id = session.sessionId;
    present.add(id);
    const decision = reconnectDecision(lastStates.get(id), session, attemptsMade.has(id));
    lastStates.set(id, session.state);
    if (session.state !== 'error') cancelCountdown(id);
    if (decision === 'reset') attemptsMade.delete(id);
    if (decision === 'schedule') void scheduleIfEnabled(id);
  }
  // A tab that is gone - closed by the user, replaced by a reconnect, or cleared by a vault lock -
  // takes its countdown with it.
  for (const id of [...lastStates.keys()]) {
    if (!present.has(id)) forget(id);
  }
}

async function scheduleIfEnabled(id: string): Promise<void> {
  const settings = await getSettings();
  // null means the read failed or the vault is locked; the setting ships on, so only an explicit
  // false turns it off.
  if (settings?.autoReconnect === false) return;
  // The settings read is asynchronous: the tab may have been closed, or recovered, meanwhile.
  if (lastStates.get(id) !== 'error' || timers.has(id)) return;

  const attempt = (attemptsMade.get(id) ?? 0) + 1;
  const delay = reconnectDelaySeconds(attempt) * 1000;
  timers.set(id, clock.setTimeout(() => void fire(id, attempt), delay));
  reconnectCountdowns.update((m) => ({ ...m, [id]: { attempt, deadline: clock.now() + delay } }));
}

async function fire(id: string, attempt: number): Promise<void> {
  cancelCountdown(id);
  if (inFlight.has(id)) return;
  inFlight.add(id);
  try {
    const replaced = await reconnectSession(id, {
      preserveTerminal: await preserveTerminalEnabled(),
      onOpened: (next) => attemptsMade.set(next, attempt),
    });
    // Opening itself failed - the connection was deleted, say - and was reported to the user.
    // That is not a network condition a timer will fix, so the chain ends here.
    if (!replaced) attemptsMade.delete(id);
  } finally {
    inFlight.delete(id);
  }
}

async function preserveTerminalEnabled(): Promise<boolean> {
  const settings = await getSettings();
  return settings?.preserveTerminalContext !== false;
}

/**
 * The Reconnect button. During a countdown it fires the pending attempt now, keeping its place in
 * the chain; otherwise it is a one-off reconnect that starts no chain of its own, so retrying a
 * failed first connect by hand never turns into retrying it on a timer.
 */
export function reconnectNow(id: string): void {
  const pending = get(reconnectCountdowns)[id];
  if (pending) {
    void fire(id, pending.attempt);
    return;
  }
  if (inFlight.has(id)) return;
  inFlight.add(id);
  void preserveTerminalEnabled()
    .then((preserveTerminal) => reconnectSession(id, { preserveTerminal }))
    .finally(() => inFlight.delete(id));
}

/** Stops the automatic attempts for one session, leaving it on screen with a plain Reconnect button. */
export function stopAutoReconnect(id: string): void {
  cancelCountdown(id);
  attemptsMade.delete(id);
}

/**
 * Re-reads the setting after the settings dialog saved. Turning auto-reconnect off must stop the
 * countdowns already running, not only the ones that would start later.
 */
export async function refreshAutoReconnectSetting(): Promise<void> {
  const settings = await getSettings();
  if (settings?.autoReconnect !== false) return;
  for (const id of [...timers.keys()]) stopAutoReconnect(id);
}

function cancelCountdown(id: string): void {
  const handle = timers.get(id);
  if (handle === undefined) return;
  clock.clearTimeout(handle);
  timers.delete(id);
  reconnectCountdowns.update((m) => {
    const next = { ...m };
    delete next[id];
    return next;
  });
}

function forget(id: string): void {
  cancelCountdown(id);
  attemptsMade.delete(id);
  lastStates.delete(id);
}
