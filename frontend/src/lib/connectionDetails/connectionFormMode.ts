import type { ConnectionProtocol } from '../../actions/protocolActions';
import type { ConnectionDetailsDraft } from './types';

export type ConnectionFormMode = 'ssh' | 'plugin' | 'none';

export interface ConnectionFormState {
  mode: ConnectionFormMode;
  protocolDef: ConnectionProtocol | null;
}

export function resolveConnectionFormMode(
  protocolId: string,
  protocols: ConnectionProtocol[],
): ConnectionFormState {
  if (protocolId === 'ssh') {
    return { mode: 'ssh', protocolDef: protocols.find((p) => p.id === 'ssh') ?? null };
  }

  const protocolDef = protocols.find((p) => p.id === protocolId) ?? null;
  if (protocolDef?.fields?.length) {
    return { mode: 'plugin', protocolDef };
  }

  return { mode: 'none', protocolDef };
}

export function applyProtocolFieldDefaults(
  draft: ConnectionDetailsDraft,
  protocolId: string,
  protocols: ConnectionProtocol[],
): void {
  const protocolDef = protocols.find((p) => p.id === protocolId) ?? null;
  if (!protocolDef?.fields) return;

  for (const group of protocolDef.fields) {
    for (const field of group.fields) {
      if (draft.pluginFields[field.id] === undefined && field.default !== undefined) {
        draft.pluginFields[field.id] = field.default;
      }
    }
  }
  draft.pluginFields = { ...draft.pluginFields };
}

export function refreshFormModeFromDraft(
  draft: ConnectionDetailsDraft,
  protocols: ConnectionProtocol[],
): ConnectionFormState {
  return resolveConnectionFormMode(draft.protocol, protocols);
}
