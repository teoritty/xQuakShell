// When an automatic reconnect happens, and whether one happens at all. Pure: no timers, no stores,
// so the schedule and the rules can be tested as the table they are.
import type { SessionState } from '../../stores/appState';

/**
 * The wait before each automatic attempt, by attempt number. The last value repeats forever.
 *
 * It grows because a link that did not come back in five seconds is usually down for longer, and
 * hammering a recovering host - or a jump host shared with other people - helps nobody. It stops
 * growing at a minute because past that the user is waiting on the app rather than on the network.
 */
export const RECONNECT_DELAYS_SECONDS: readonly number[] = [5, 15, 30, 60];

/** The wait before attempt `attempt` (1-based). */
export function reconnectDelaySeconds(attempt: number): number {
  const index = Math.min(Math.max(Math.floor(attempt), 1), RECONNECT_DELAYS_SECONDS.length) - 1;
  return RECONNECT_DELAYS_SECONDS[index];
}

/**
 * Whole seconds left until `deadline`, rounded up, never negative.
 *
 * Rounded up so the button reads 5 for the whole first second and reaches 0 only when the attempt
 * is actually due: rounding down would show a 0 that sits on screen for most of a second.
 */
export function secondsRemaining(deadline: number, now: number): number {
  return Math.max(0, Math.ceil((deadline - now) / 1000));
}

export type ReconnectDecision = 'schedule' | 'reset' | 'none';

/** The part of a session the decision reads. */
export interface ReconnectObservation {
  state: SessionState;
  connectionLost?: boolean;
}

/**
 * What a state update means for automatic reconnect.
 *
 * - `reset`: the session is up. Whatever chain of attempts led here is over, so the next loss
 *   starts again from the shortest wait.
 * - `schedule`: the session has just failed and is worth retrying. That is either an established
 *   session whose link dropped (`connectionLost`), or a session that is itself an automatic attempt
 *   (`inChain`) - a failed attempt is a failed connect, never a lost connection, and the chain
 *   would otherwise end at its first failure.
 * - `none`: everything else. In particular a failed first connect and a shell that exited on its
 *   own: retrying a wrong password on a timer only earns a ban, and reopening a shell the user
 *   logged out of undoes what they just did.
 *
 * Only the transition into `error` schedules. The store republishes a session on every change to
 * any field, and scheduling again on each of those would stack timers for one failure.
 */
export function reconnectDecision(
  previous: SessionState | undefined,
  next: ReconnectObservation,
  inChain: boolean,
): ReconnectDecision {
  if (next.state === 'ready') return 'reset';
  if (next.state !== 'error' || previous === 'error') return 'none';
  return next.connectionLost || inChain ? 'schedule' : 'none';
}
