// Plugin-owned tabs (ADR-015) and the one lookup that unifies them with sessions.
//
// A surface sits beside a session in the tile grid but is not one: it has no connection of its
// own, no SSH client, no host-key decision. Keeping it in a separate store rather than widening
// Session is what stops those two very different things from having to share a shape.
import { writable, get } from 'svelte/store';
import { sessions, type Session } from './appState';
import { closeSurface as closeSurfaceRpc } from '../api/surfaces';
import { closeSession } from '../actions/sessionActions';

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
 * A tab is one of two things, and every consumer has to know which.
 *
 * `null` is a real answer, not an error: a tile can name a tab id for a moment after the thing
 * behind it closed, and rendering nothing is the correct response.
 */
export type Tab =
  | { kind: 'session'; session: Session }
  | { kind: 'surface'; surface: Surface }
  | null;

export function resolveTab(id: string): Tab {
  const session = get(sessions).find((s) => s.sessionId === id);
  if (session) return { kind: 'session', session };
  const surface = get(surfaces).find((s) => s.surfaceId === id);
  if (surface) return { kind: 'surface', surface };
  return null;
}

/** Title shown on the tab, for either kind. */
export function tabTitle(tab: Tab): string {
  if (!tab) return '';
  return tab.kind === 'session' ? tab.session.connectionName || 'Session' : tab.surface.title || 'Surface';
}

/** State shown as the tab's status dot, for either kind. */
export function tabState(tab: Tab): string {
  if (!tab) return '';
  return tab.kind === 'session' ? tab.session.state : tab.surface.state;
}

/**
 * Closes whatever a tab id names.
 *
 * The two closes are genuinely different calls — one ends an SSH session, the other releases a
 * plugin's tab — and the tab bar must not be the place that knows which. Ids are disjoint by
 * construction (surface ids are minted with an `srf-` prefix), so the lookup cannot pick wrong.
 */
export async function closeTab(id: string): Promise<void> {
  const tab = resolveTab(id);
  if (!tab) return;
  if (tab.kind === 'session') {
    await closeSession(id);
    return;
  }
  await closeSurfaceRpc(id);
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
