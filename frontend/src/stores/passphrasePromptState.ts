import { writable } from 'svelte/store';

/**
 * A connecting session waiting for the user to open one of its keys.
 *
 * It names the key and nothing else. The passphrase travels only the other way, as the argument of
 * ResolvePassphrase, and is never stored here: this queue lives as long as the app, and a secret in
 * it would outlive the dialog it was typed into.
 */
export interface PassphrasePromptEvent {
  requestId: string;
  identityId: string;
  label: string;
  /** The passphrase just typed for this key did not open it; the dialog says so. */
  retry?: boolean;
}

/**
 * Passphrase questions waiting for an answer, oldest first.
 *
 * A queue for the same reason pendingPeerTrust is one: two sessions can each be waiting on a key,
 * and a single slot would let the second prompt overwrite the first, leaving its connection stuck
 * in "connecting" behind a dialog that no longer exists.
 */
export const pendingPassphrasePrompts = writable<PassphrasePromptEvent[]>([]);

/** Queue a prompt. A repeat of a queued request id replaces it rather than stacking a duplicate. */
export function enqueuePassphrasePrompt(prompt: PassphrasePromptEvent): void {
  pendingPassphrasePrompts.update((queue) => [...queue.filter((q) => q.requestId !== prompt.requestId), prompt]);
}

/**
 * Drop a prompt: answered, cancelled, or withdrawn by the backend because its session ended. The
 * backend's PassphrasePromptClosed is what makes the last case work - the frontend has no other way
 * to learn that the connection behind a dialog has gone.
 */
export function dropPassphrasePrompt(requestId: string): void {
  pendingPassphrasePrompts.update((queue) => queue.filter((q) => q.requestId !== requestId));
}
