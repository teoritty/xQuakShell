import { writable, type Readable } from 'svelte/store';
import { PING_RESULT_TTL_MS, pingFailed, pingSucceeded, type PingOutcome } from './pingView';

/**
 * Holds the most recent ping per plugin and retires each one on a timer.
 *
 * Its own module because it is the only stateful part of the ping, and the dialog it used to live
 * in is a container already at its size budget. Keeping it here also makes the two rules that are
 * easy to get wrong - a second ping must not be retired by the first one's timer, and a closed
 * dialog must not leave timers running - testable without mounting anything.
 *
 * The ping call is passed in rather than imported so this module owes nothing to the api layer; the
 * component that has the gateway supplies it.
 */
export interface PingTracker extends Readable<Record<string, PingOutcome>> {
  /** Pings one plugin and records the answer, whichever kind of answer it is. */
  run(pluginId: string, ping: (pluginId: string) => Promise<Record<string, string>>): Promise<void>;
  /** Drops every pending timer. Call from onDestroy. */
  dispose(): void;
}

export interface PingTrackerOptions {
  /** How long a result stays before the row goes back to run state. */
  ttlMs?: number;
  /** The clock, so a test can measure a fixed interval instead of a real one. */
  now?: () => number;
}

export function createPingTracker(options: PingTrackerOptions = {}): PingTracker {
  const ttlMs = options.ttlMs ?? PING_RESULT_TTL_MS;
  const now = options.now ?? (() => performance.now());

  const results = writable<Record<string, PingOutcome>>({});
  const timers = new Map<string, ReturnType<typeof setTimeout>>();

  function record(pluginId: string, outcome: PingOutcome): void {
    // A second ping while the first result is still showing must not be retired by the first
    // one's timer, or the newer answer vanishes early and looks like a flicker.
    const running = timers.get(pluginId);
    if (running) clearTimeout(running);

    results.update((current) => ({ ...current, [pluginId]: outcome }));
    timers.set(
      pluginId,
      setTimeout(() => {
        timers.delete(pluginId);
        results.update((current) => {
          const { [pluginId]: _retired, ...rest } = current;
          return rest;
        });
      }, ttlMs),
    );
  }

  return {
    subscribe: results.subscribe,

    async run(pluginId, ping) {
      const started = now();
      try {
        // The payload is awaited into a variable first, deliberately. Written as
        // `pingSucceeded(now() - started, await ping(id))` the arguments evaluate left to right,
        // so the clock is read before the call is even made and every ping reports 0 ms.
        const payload = await ping(pluginId);
        record(pluginId, pingSucceeded(now() - started, payload));
      } catch (e) {
        // Caught, never rethrown. Both outcomes are answers to the question the user asked, and
        // the caller has nothing useful to do with a rejection it would only have to swallow.
        record(pluginId, pingFailed(now() - started, e));
      }
    },

    dispose() {
      for (const timer of timers.values()) clearTimeout(timer);
      timers.clear();
      results.set({});
    },
  };
}
