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
  // Protocol controls how the connection is opened, not which reversible draft fields
  // are retained. SSH users and jump hosts are persisted even when a plugin protocol
  // is selected so autosave cannot destroy credential references during protocol
  // switching. Domain connect validation is responsible for ignoring these fields
  // for non-SSH protocols.
  return {
    id: draft.editingId,
    name: draft.name.trim() || 'New connection',
    protocol: draft.protocol,
    host: draft.host.trim(),
    port: draft.port,
    folderId: context.folderId,
    tags: [...draft.tags],
    users: filterDraftUsers(draft.users),
    defaultUserId: draft.defaultUserId,
    jumpChain: stripDraftHopIdsForSave(filterDraftHops(draft.jumpHops)),
    order: context.order,
  };
}
