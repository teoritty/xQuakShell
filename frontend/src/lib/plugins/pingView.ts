// What a ping looks like once it has happened. Kept out of the component so the rules can be
// tested without mounting one, and so the dialog holds outcomes rather than deciding what they mean.

/** How long a result stays on the card before the row goes back to showing run state. */
export const PING_RESULT_TTL_MS = 6000;

export interface PingOutcome {
  ok: boolean;
  /**
   * Round trip in whole milliseconds, measured in the UI.
   *
   * The UI's number and not the backend's on purpose: what the user is asking is "is this plugin
   * answering me", and the answer includes the RPC hop and the plugin's own scheduling. A number
   * measured inside the host would be smaller and less true.
   */
  ms: number;
  /** The pong payload on success, the backend's own message on failure. Shown as the tooltip. */
  detail: string;
}

/**
 * Renders the pong payload as one line.
 *
 * Plugins answer `{"pong":"ok"}` at minimum and may add whatever else they want to report, so this
 * cannot assume a shape. Keys are sorted because the payload arrives as a JSON object and object
 * key order is not something to show a user twice in a row differently.
 */
export function formatPong(payload: Record<string, string>): string {
  const keys = Object.keys(payload).sort();
  if (keys.length === 0) return '';
  return keys.map((k) => `${k}=${payload[k]}`).join(' ');
}

/** The outcome of a ping that answered. */
export function pingSucceeded(ms: number, payload: Record<string, string>): PingOutcome {
  return { ok: true, ms: roundMs(ms), detail: formatPong(payload) };
}

/**
 * The outcome of a ping that did not answer.
 *
 * The backend's message is carried through verbatim rather than classified into cases here. The
 * reasons a ping fails - the process is not running, the call timed out, the pipe is broken - reach
 * the frontend only as an error string, and matching on that string is how the docker plugin ended
 * up in a retry loop against wording that had changed. What the user needs is the reason, and the
 * reason is what the backend already wrote.
 */
export function pingFailed(ms: number, error: unknown): PingOutcome {
  return { ok: false, ms: roundMs(ms), detail: messageOf(error) };
}

/**
 * The status line for a card whose ping has just finished: a message key and its values.
 *
 * Keys rather than text, because this module is not allowed to know which language is loaded.
 */
export function pingStatus(outcome: PingOutcome): {
  key: string;
  values: Record<string, string | number>;
  kind: 'installed' | 'warning';
} {
  return outcome.ok
    ? { key: 'plugins.ping.ok', values: { ms: outcome.ms }, kind: 'installed' }
    : { key: 'plugins.ping.failed', values: {}, kind: 'warning' };
}

// Sub-millisecond round trips are real - a plugin on the same machine over a pipe answers in well
// under one - and "0 ms" reads as a failure to measure rather than as a fast answer. Anything that
// took any time at all is reported as at least one.
function roundMs(ms: number): number {
  if (!Number.isFinite(ms) || ms <= 0) return 0;
  return Math.max(1, Math.round(ms));
}

function messageOf(error: unknown): string {
  if (error instanceof Error) return error.message;
  if (typeof error === 'string') return error;
  return String(error);
}
