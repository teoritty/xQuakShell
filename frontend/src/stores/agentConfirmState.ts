import { writable } from 'svelte/store';

/**
 * A forwarded-agent signature request waiting for the user (ADR-022).
 *
 * It carries names and fingerprints only. The data the server wants signed never leaves the
 * backend: the user decides on who asks, for which key, as whom and where the signature goes.
 */
export interface AgentConfirmEvent {
  requestId: string;
  sessionId: string;
  connectionName: string;
  host: string;
  keyLabel: string;
  keyFingerprint: string;
  remoteUser: string;
  /** True when the remote client proved the destination with session-bind. */
  verified: boolean;
  /** Destination host key fingerprint; empty when unverified. */
  destination: string;
  /** known_hosts names carrying that key. */
  knownAs: string[];
  forwardsFurther: boolean;
  /** The window the dialog may offer, in minutes; 0 offers none. */
  windowMinutes: number;
}

/**
 * Requests waiting for an answer, oldest first. A queue for the reason the passphrase prompts are
 * one: several sessions can each be waiting, and a single slot would drop all but the last.
 */
export const pendingAgentConfirms = writable<AgentConfirmEvent[]>([]);

/**
 * Queue a request. Anything malformed is dropped rather than shown: a dialog that cannot say who
 * is asking is a dialog the user cannot answer safely, and the backend denies it on timeout.
 */
export function enqueueAgentConfirm(e: AgentConfirmEvent | null | undefined): void {
  if (!e?.requestId || !e.sessionId) return;
  const event: AgentConfirmEvent = {
    ...e,
    knownAs: Array.isArray(e.knownAs) ? e.knownAs : [],
    verified: e.verified === true,
    forwardsFurther: e.forwardsFurther === true,
    windowMinutes: e.verified === true && e.windowMinutes > 0 ? e.windowMinutes : 0,
  };
  pendingAgentConfirms.update((q) => [...q.filter((x) => x.requestId !== event.requestId), event]);
}

/** Drop a request: answered here, or withdrawn by the backend (timeout, session closed). */
export function dropAgentConfirm(requestId: string): void {
  pendingAgentConfirms.update((q) => q.filter((x) => x.requestId !== requestId));
}
