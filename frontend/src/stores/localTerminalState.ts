// Local shell tabs: the third kind of thing that can occupy a tile.
//
// A third store rather than a widened Session or Surface, for the reason surfaceState.ts already
// gives about surfaces: a local terminal has no connection, no vault binding and no host key, and
// sharing a shape with something that does is how code starts treating them as interchangeable.
//
// Note what this file does NOT export: there is no clearLocalTerminals(). Surfaces have one and
// events/subscribe.ts calls it when the vault locks, because a plugin's tabs belong to sessions
// that no longer exist. A local shell belongs to the user's own machine and holds nothing from
// the vault, so locking hides it rather than killing it - and a lock that killed a running build
// would be the worst surprise this feature could produce. The absence of the function is what
// keeps that true.
import { writable, get } from 'svelte/store';

export interface LocalTerminal {
  id: string;
  title: string;
}

export const localTerminals = writable<LocalTerminal[]>([]);

/** Adds or replaces a terminal. Replacement keeps the store idempotent under a repeated event. */
export function upsertLocalTerminal(terminal: LocalTerminal): void {
  localTerminals.update((list) => {
    const idx = list.findIndex((t) => t.id === terminal.id);
    if (idx < 0) return [...list, terminal];
    const next = [...list];
    next[idx] = terminal;
    return next;
  });
}

export function removeLocalTerminal(id: string): void {
  localTerminals.update((list) => list.filter((t) => t.id !== id));
}

/** Every open local terminal id, for the imperative callers that cannot read the store. */
export function localTerminalIds(): string[] {
  return get(localTerminals).map((t) => t.id);
}
