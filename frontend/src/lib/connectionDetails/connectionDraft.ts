import type { Connection } from '../../stores/appState';
import type { ConnectionDetailsDraft } from './types';
import type { ConnectionProtocol } from '../../stores/api';
import { ensureHopUiId } from './hopIds';

export function createDraftFromConnection(
  connection: Connection | null | undefined,
  defaultPort: number,
): ConnectionDetailsDraft {
  return {
    editingId: connection?.id || '',
    name: connection?.name || '',
    protocol: connection?.protocol || 'ssh',
    host: connection?.host || '',
    port: connection?.port || defaultPort,
    tags: [...(connection?.tags || [])],
    users: (connection?.users || []).map((u) => ({ ...u })),
    defaultUserId: connection?.defaultUserId || '',
    jumpHops: (connection?.jumpChain || []).map((h, i) => ensureHopUiId({ ...h }, i)),
  };
}

export function resolveDefaultPort(
  protocol: string,
  protocols: ConnectionProtocol[],
  connectionPort?: number,
): number {
  if (connectionPort) return connectionPort;
  return protocols.find((p) => p.id === protocol)?.defaultPort || 22;
}
