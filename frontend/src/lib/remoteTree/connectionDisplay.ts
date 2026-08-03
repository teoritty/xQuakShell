import type { Session } from '../../stores/appState';
import { statusDotColor, statusDotTooltip, type StatusDot } from './statusDot';
import type { ConnectionStatus } from './types';

type PingMap = Map<string, { reachable?: boolean; latencyMs?: number }>;

// hasPingResult reports whether a ping result exists yet for the connection.
// When false the host has not been pinged yet (a ping is pending/in progress),
// which the UI renders as a spinner instead of a dot.
export function hasPingResult(pingMap: PingMap, connId: string): boolean {
  return pingMap.has(connId);
}

/**
 * The ping expressed as an ordinary StatusDot, so the connection row and a
 * discovery row draw the same primitive.
 *
 * `null` means "no ping result yet" — the view renders a spinner, not a dot.
 * The thresholds (100/300/1000 ms), the grey-not-red unreachable colour and the
 * tooltip strings are unchanged from the hand-rolled version and are pinned by
 * pingCharacterization.test.ts.
 *
 * The 300–999 ms band uses an explicit `color`: it is a fourth step that has no
 * tone of its own, and #ff6f00 is a valid six-digit hex, so it passes the same
 * override validation a plugin's colour would.
 */
export function pingStatus(pingMap: PingMap, connId: string): StatusDot | null {
  const r = pingMap.get(connId);
  if (!r) return null;
  if (!r.reachable) return { tone: 'neutral', tooltip: 'Unreachable' };
  const tooltip = `${r.latencyMs}ms`;
  const latency = r.latencyMs ?? 0;
  if (latency < 100) return { tone: 'ok', tooltip };
  if (latency < 300) return { tone: 'warn', tooltip };
  if (latency < 1000) return { tone: 'warn', color: '#ff6f00', tooltip };
  return { tone: 'error', tooltip };
}

/**
 * Kept as a thin wrapper over pingStatus + statusDotColor rather than deleted:
 * pingCharacterization.test.ts calls it, and a characterization test that has to
 * be rewritten to survive the refactor it characterizes is worth nothing.
 */
export function pingColor(pingMap: PingMap, connId: string): string {
  return statusDotColor(pingStatus(pingMap, connId));
}

export function pingTooltip(pingMap: PingMap, connId: string): string {
  const status = pingStatus(pingMap, connId);
  if (!status) return 'Not pinged yet';
  return statusDotTooltip(status);
}

export function tagColor(tag: string): string {
  let hash = 0;
  for (let i = 0; i < tag.length; i++) {
    hash = tag.charCodeAt(i) + ((hash << 5) - hash);
  }
  const h = Math.abs(hash) % 360;
  return `hsl(${h}, 50%, 40%)`;
}

export function buildSessionStatusMap(sessions: Session[]): Map<string, ConnectionStatus> {
  const m = new Map<string, ConnectionStatus>();
  for (const s of sessions) {
    const st: ConnectionStatus =
      s.state === 'ready'
        ? 'active'
        : s.state === 'connecting'
          ? 'connecting'
          : s.state === 'error'
            ? 'error'
            : 'disconnected';
    m.set(s.connectionId, st);
  }
  return m;
}
