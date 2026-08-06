// What the user can do to a tab, whichever kind it is (ADR-015).
//
// A tab id names either an SSH session or a plugin-owned surface, and the two are closed by
// genuinely different calls. Every entry point that acts on "the tab" — the tab bar's close
// button, the close and cycle hotkeys — routes through here, so none of them has to know the
// difference and none of them can get it wrong. The stores hold state; this file is the only
// place that decides what an action means.
import { get } from 'svelte/store';
import { activeTabId, sessions } from '../stores/appState';
import { surfaces, resolveTab } from '../stores/surfaceState';
import { closeSession } from './sessionActions';
import { closeSurface as closeSurfaceRpc } from '../api/surfaces';

/**
 * Every open tab id, sessions first.
 *
 * Sessions first keeps the cycle order unchanged for a window with no plugin tabs in it, which is
 * every window until a plugin opens one.
 */
export function allTabIds(): string[] {
  return [
    ...get(sessions).map((s) => s.sessionId),
    ...get(surfaces).map((s) => s.surfaceId),
  ];
}

/**
 * Closes whatever a tab id names.
 *
 * Ids are disjoint by construction — surface ids are minted with an `srf-` prefix — so the lookup
 * cannot pick wrong. An id that names nothing is a no-op: the tab may have closed a moment ago.
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

/**
 * Closes the focused tab and moves focus to another one.
 *
 * This is the hotkey path. It used to call closeSession directly, which for a surface id reached
 * the backend as a session that does not exist: the error was swallowed as "session not found",
 * the plugin's tab stayed open, and focus jumped away from it anyway.
 */
export async function closeActiveTab(): Promise<void> {
  const currentId = get(activeTabId);
  if (!currentId) return;
  await closeTab(currentId);
  const remaining = allTabIds().filter((id) => id !== currentId);
  activeTabId.set(remaining.length > 0 ? remaining[remaining.length - 1] : '');
}

export function focusNextTab(): void {
  cycleTab(1);
}

export function focusPrevTab(): void {
  cycleTab(-1);
}

function cycleTab(direction: 1 | -1): void {
  const ids = allTabIds();
  if (ids.length === 0) return;
  const currentIdx = Math.max(0, ids.indexOf(get(activeTabId)));
  const nextIdx = (currentIdx + direction + ids.length) % ids.length;
  activeTabId.set(ids[nextIdx]);
}
