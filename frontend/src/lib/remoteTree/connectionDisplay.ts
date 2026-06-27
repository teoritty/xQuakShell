import type { Session } from '../../stores/appState';
import type { ConnectionStatus } from './types';

export function pingColor(pingMap: Map<string, { reachable?: boolean; latencyMs?: number }>, connId: string): string {
  const r = pingMap.get(connId);
  if (!r) return 'transparent';
  if (!r.reachable) return 'var(--danger, #f44747)';
  if ((r.latencyMs ?? 0) < 100) return '#4caf50';
  if ((r.latencyMs ?? 0) < 300) return '#ffb300';
  if ((r.latencyMs ?? 0) < 1000) return '#ff6f00';
  return 'var(--danger, #f44747)';
}

export function pingTooltip(pingMap: Map<string, { reachable?: boolean; latencyMs?: number }>, connId: string): string {
  const r = pingMap.get(connId);
  if (!r) return 'Not pinged yet';
  if (!r.reachable) return 'Unreachable';
  return `${r.latencyMs}ms`;
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
