// frontend/src/lib/terminalPool.ts
// Keeps one xterm instance alive per session for as long as the session exists,
// independent of the Terminal.svelte component's mount lifecycle.
//
// Why: SessionView (and its Terminal) is mounted inside a tile. When a session
// is dragged to another tile, split out, reoriented or swapped, its SessionView
// is destroyed in the old tile component and recreated in the new one — which,
// with a per-component xterm, would create a fresh terminal and lose all
// scrollback. Pooling the xterm (and the host element it is opened on) lets a
// remounting Terminal re-attach the SAME live terminal, so the buffer, cursor
// and PTY wiring survive every layout change. The terminal is disposed only when
// the session is actually closed (see disposeTerminal, called from the session
// lifecycle in stores/api).

import type { Terminal } from '@xterm/xterm';
import type { FitAddon } from '@xterm/addon-fit';

export interface PooledTerminal {
  term: Terminal;
  fitAddon: FitAddon;
  /** The element xterm was opened on. Moved between containers across remounts. */
  host: HTMLDivElement;
  /** Unscaled font size from settings, so a remount keeps UI-scale math correct. */
  baseFontSize: number;
}

const pool = new Map<string, PooledTerminal>();

/** Returns the live terminal for a session, or undefined if none exists yet. */
export function getPooledTerminal(sessionId: string): PooledTerminal | undefined {
  return pool.get(sessionId);
}

/** Stores a freshly created terminal so later remounts can reuse it. */
export function setPooledTerminal(sessionId: string, pooled: PooledTerminal): void {
  pool.set(sessionId, pooled);
}

/**
 * Permanently tears down a session's terminal. Call this only when the session
 * is closed — never on a mere component unmount, which is expected during tile
 * rearrangements.
 */
export function disposeTerminal(sessionId: string): void {
  const pooled = pool.get(sessionId);
  if (!pooled) return;
  pool.delete(sessionId);
  try {
    pooled.term.dispose();
  } catch {
    // already disposed
  }
  if (pooled.host.parentNode) pooled.host.parentNode.removeChild(pooled.host);
}
