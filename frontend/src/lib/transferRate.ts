// Presentation-layer estimation of a transfer's byte rate from successive
// progress samples. Framework-free and unit-testable.
//
// Speed is inherently a display-derived value, so it is computed here rather
// than in the Go transfer orchestrator, which stays a pure transfer service.

// Smoothing factor for the exponential moving average. Higher = more
// responsive but jitterier; 0.3 balances responsiveness against stability.
const EMA_ALPHA = 0.3;

interface RateState {
  lastDone: number;
  lastAt: number; // milliseconds
  rate: number; // smoothed bytes/sec
}

export interface RateTracker {
  /**
   * Record a progress sample and return the current smoothed rate (bytes/sec).
   * `done` is cumulative bytes transferred; `now` is a millisecond timestamp.
   * The first sample for an id establishes a baseline and returns 0.
   */
  sample(id: string, done: number, now: number): number;
  /** Drop all state for an id (call on any terminal transfer state). */
  clear(id: string): void;
}

export function createRateTracker(): RateTracker {
  const states = new Map<string, RateState>();

  return {
    sample(id, done, now) {
      const prev = states.get(id);
      if (!prev) {
        states.set(id, { lastDone: done, lastAt: now, rate: 0 });
        return 0;
      }

      const dt = (now - prev.lastAt) / 1000;
      const dBytes = done - prev.lastDone;

      // Guard against a stale/duplicate sample (dt <= 0) and against the
      // terminal event resetting `done` to 0 (dBytes < 0): keep the last
      // rate, but still advance the reference point on a real timestamp.
      if (dt <= 0 || dBytes < 0) {
        prev.lastDone = done;
        if (now > prev.lastAt) prev.lastAt = now;
        return prev.rate;
      }

      const instant = dBytes / dt;
      prev.rate = prev.rate === 0 ? instant : EMA_ALPHA * instant + (1 - EMA_ALPHA) * prev.rate;
      prev.lastDone = done;
      prev.lastAt = now;
      return prev.rate;
    },

    clear(id) {
      states.delete(id);
    },
  };
}
