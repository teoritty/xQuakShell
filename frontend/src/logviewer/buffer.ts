/**
 * The debug log window's ring of lines.
 *
 * It is a module rather than three statements inside the component because what it does is the
 * part that has to be right under load, and a Svelte component is not where that can be tested.
 *
 * The window receives one event per log record and can receive thousands a second. Growing the
 * ring one record at a time made every one of them a full array copy plus a full re-render, which
 * saturated the UI thread and left the toolbar unpainted - the level selector rendered as a
 * transparent hole while lines poured in. Lines are therefore batched by the caller and appended
 * a frame's worth at a time, through here.
 */

/** A log line carrying the identity the keyed `{#each}` uses to reuse its DOM row. */
export interface Sequenced {
  seq: number;
}

/**
 * Appends a batch to the ring and keeps only the newest `max`.
 *
 * Returns `existing` **unchanged, by reference** for an empty batch. That is the point of the
 * function rather than a micro-optimisation: assigning a fresh array in Svelte invalidates it, so
 * returning a copy here would re-render the whole list on every frame that happened to carry no
 * new lines.
 */
export function appendCapped<T>(existing: readonly T[], batch: readonly T[], max: number): T[] {
  if (batch.length === 0) return existing as T[];
  if (max <= 0) return [];
  const combined = existing.concat(batch as T[]);
  return combined.length > max ? combined.slice(combined.length - max) : combined;
}

/**
 * Caps the not-yet-flushed batch.
 *
 * requestAnimationFrame does not fire while the window is minimised or hidden, so without this
 * the pending batch of a chatty session grows for as long as the user looks away - an unbounded
 * buffer behind a bounded ring. Dropping the oldest here loses exactly the lines the ring would
 * have dropped at flush time anyway.
 */
export function capPending<T>(pending: T[], max: number): T[] {
  if (max <= 0) return [];
  return pending.length > max ? pending.slice(pending.length - max) : pending;
}
