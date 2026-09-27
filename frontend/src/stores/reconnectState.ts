import { writable } from 'svelte/store';

/** A pending automatic reconnect: which attempt it will be, and when it fires (epoch ms). */
export interface ReconnectCountdown {
  attempt: number;
  deadline: number;
}

/**
 * Pending automatic reconnects by session id. Written only by actions/reconnectActions.ts, which
 * owns the timers; components read it to draw the countdown in the Reconnect button.
 *
 * A deadline rather than a ticking number of seconds, so the store changes twice per attempt
 * instead of once a second, and a countdown is never wrong because a tick was late.
 */
export const reconnectCountdowns = writable<Record<string, ReconnectCountdown>>({});
