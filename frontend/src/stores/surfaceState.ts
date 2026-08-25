// Plugin-owned tabs (ADR-015) and the one lookup that unifies them with sessions.
//
// A surface sits beside a session in the tile grid but is not one: it has no connection of its
// own, no SSH client, no host-key decision. Keeping it in a separate store rather than widening
// Session is what stops those two very different things from having to share a shape.
//
// State and pure lookups only: what to DO with a tab — close it, cycle to the next one — lives in
// actions/tabActions.ts, so this store stays free of the RPC layer (§1.5).
import { writable, derived, get } from 'svelte/store';
import { sessions, type Session } from './appState';
import { localTerminals, type LocalTerminal } from './localTerminalState';

export type SurfaceKind = 'terminal' | 'log';
export type SurfaceState = 'connecting' | 'ready' | 'error';

export interface Surface {
  surfaceId: string;
  connectionId: string;
  pluginId: string;
  kind: SurfaceKind;
  title: string;
  iconId: string;
  state: SurfaceState;
  errorMessage: string;
}

export const surfaces = writable<Surface[]>([]);

/**
 * Whether anything at all is open, across every kind of tab.
 *
 * A derived store rather than a check in the template, because the template got it wrong: the
 * tile grid was gated on the SSH session count, so opening a local terminal with nothing else
 * open started a real shell behind an unchanged welcome screen. Nothing errored and nothing was
 * logged - the tab existed, the shell ran, and no component was mounted to show it.
 *
 * A plugin surface could never expose that bug: a surface borrows a session's authorization and
 * cannot exist without one (ADR-015), so a surface was never the only tab. A local terminal can
 * be, which is why the question has to be "is any tab open" rather than "is any session open".
 *
 * Here rather than in a component so it can be tested, which the template condition could not be.
 */
export const hasOpenTabs = derived(
  [sessions, surfaces, localTerminals],
  ([$sessions, $surfaces, $localTerminals]) =>
    $sessions.length > 0 || $surfaces.length > 0 || $localTerminals.length > 0
);

/**
 * A tab is one of two things, and every consumer has to know which.
 *
 * `null` is a real answer, not an error: a tile can name a tab id for a moment after the thing
 * behind it closed, and rendering nothing is the correct response.
 */
export type Tab =
  | { kind: 'session'; session: Session }
  | { kind: 'surface'; surface: Surface }
  | { kind: 'local'; local: LocalTerminal }
  | null;

/**
 * Resolves an id against explicit lists.
 *
 * Components use this rather than resolveTab so their reactive statements name $sessions and
 * $surfaces as real arguments: a lookup that reads the stores itself is invisible to the compiler,
 * and the tab bar would then not repaint when a surface's title changed.
 */
export function resolveTabIn(
  sessionList: Session[],
  surfaceList: Surface[],
  localList: LocalTerminal[],
  id: string
): Tab {
  const session = sessionList.find((s) => s.sessionId === id);
  if (session) return { kind: 'session', session };
  const surface = surfaceList.find((s) => s.surfaceId === id);
  if (surface) return { kind: 'surface', surface };
  const local = localList.find((t) => t.id === id);
  if (local) return { kind: 'local', local };
  return null;
}

/** The same lookup for imperative callers, which have no reactive context to feed. */
export function resolveTab(id: string): Tab {
  return resolveTabIn(get(sessions), get(surfaces), get(localTerminals), id);
}

/** Title shown on the tab, for either kind. */
export function tabTitle(tab: Tab): string {
  if (!tab) return '';
  if (tab.kind === 'session') return tab.session.connectionName || 'Session';
  if (tab.kind === 'local') return tab.local.title || 'shell';
  return tab.surface.title || 'Surface';
}

/** State shown as the tab's status dot, for either kind. */
export function tabState(tab: Tab): string {
  if (!tab) return '';
  if (tab.kind === 'session') return tab.session.state;
  // A local shell has no connecting phase and no remote end that can refuse, so it is only ever
  // ready. Reporting anything else would put a status dot on a tab with no status to report.
  if (tab.kind === 'local') return 'ready';
  return tab.surface.state;
}

/** Adds or replaces a surface. Replacement keeps the store idempotent under a repeated event. */
export function upsertSurface(surface: Surface): void {
  surfaces.update((list) => {
    const idx = list.findIndex((s) => s.surfaceId === surface.surfaceId);
    if (idx < 0) return [...list, surface];
    const next = [...list];
    next[idx] = surface;
    return next;
  });
}

export function removeSurface(surfaceId: string): void {
  surfaces.update((list) => list.filter((s) => s.surfaceId !== surfaceId));
}

/** Drops every surface. Used on vault lock, where nothing plugin-owned may stay on screen. */
export function clearSurfaces(): void {
  surfaces.set([]);
}
