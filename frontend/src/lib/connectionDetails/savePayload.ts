import type { ConnectionUser } from '../../stores/appState';
import type { ConnectionDetailsDraft } from './types';
import { filterDraftHops, stripDraftHopIdsForSave } from './hopIds';

export function filterDraftUsers(users: ConnectionUser[]): ConnectionUser[] {
  let filtered = users.filter(
    (u) =>
      u.username.trim() !== '' ||
      (u.authMethod === 'password' && u.passAuth?.passwordId) ||
      (u.keyAuth?.identityIds && u.keyAuth.identityIds.length > 0),
  );
  if (filtered.length === 0 && users.length > 0) {
    filtered = [...users];
  }
  return filtered;
}

export interface ConnectionSaveContext {
  folderId: string;
  order: number;
}

export function buildConnectionSavePayload(
  draft: ConnectionDetailsDraft,
  context: ConnectionSaveContext,
): Record<string, unknown> {
  const isSSH = draft.protocol === 'ssh';
  return {
    id: draft.editingId,
    name: draft.name.trim() || 'New connection',
    protocol: draft.protocol,
    host: draft.host.trim(),
    port: draft.port,
    folderId: context.folderId,
    tags: [...draft.tags],
    users: isSSH ? filterDraftUsers(draft.users) : [],
    defaultUserId: isSSH ? draft.defaultUserId : '',
    jumpChain: isSSH ? stripDraftHopIdsForSave(filterDraftHops(draft.jumpHops)) : [],
    order: context.order,
  };
}
