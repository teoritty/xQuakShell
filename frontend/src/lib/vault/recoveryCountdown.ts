// The delay on the Done button of the one-time recovery key dialog.
//
// Deliberately import-free and side-effect-free, so it stays outside every rule in
// src/architecture.test.ts and can be tested without rendering a component.

// RECOVERY_ACKNOWLEDGE_SECONDS is how long Done stays disabled.
//
// The dialog is the only time the key is ever shown, and pressing Done destroys the only copy the
// application holds. Fifteen seconds is long enough that nobody clicks it reflexively while
// reaching for the next window, and short enough that someone who has already written the key down
// is not left staring at a screen they are finished with.
export const RECOVERY_ACKNOWLEDGE_SECONDS = 15;

// tickCountdown returns the next value of the counter, never going below zero.
//
// Clamping here rather than in the component is what keeps a timer that fires one extra time from
// rendering "Done (-1)".
export function tickCountdown(secondsLeft: number): number {
  if (!Number.isFinite(secondsLeft) || secondsLeft <= 1) return 0;
  return Math.floor(secondsLeft) - 1;
}

// isAcknowledgeEnabled reports whether Done may be pressed.
//
// A non-finite or negative value counts as elapsed rather than as blocked forever: a broken timer
// must not be able to trap someone in a dialog that cannot be closed any other way.
export function isAcknowledgeEnabled(secondsLeft: number): boolean {
  return !Number.isFinite(secondsLeft) || secondsLeft <= 0;
}

// acknowledgeLabel renders the button text, counting down inside it.
export function acknowledgeLabel(done: string, secondsLeft: number): string {
  return isAcknowledgeEnabled(secondsLeft) ? done : `${done} (${Math.ceil(secondsLeft)})`;
}
