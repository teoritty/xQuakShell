// Drag and drop across synced vaults. Inside one vault a drop is a folder change and a reorder, as
// it always was. Across vaults it is a move - a different operation, which shares the host with
// other people or takes it away from them - so it is never done as a folder change: the backend
// refuses that, and the drop asks the person to confirm the move instead.
import type { Connection, Folder } from '../../stores/appState';
import { requestVaultMove } from '../../stores/teamState';
import type { DragPayload, TreeNode } from './types';

/**
 * What a drop into targetVault does: 'same' when everything dragged is already there; 'move' when
 * only connections are dragged and all of them come from elsewhere; 'refuse' otherwise. A folder
 * does not move between vaults - its contents would each need the move's credential handling - and
 * a drag mixing items of the target with items from elsewhere has no single meaning.
 */
export function classifyVaultDrop(
  payload: DragPayload,
  targetVault: string,
  connections: Connection[],
  folders: Folder[]
): 'same' | 'move' | 'refuse' {
  const foreign = (vault: string | undefined) => (vault ?? '') !== targetVault;
  const foreignFolders = folders.filter((f) => payload.folderIds.includes(f.id) && foreign(f.vaultId)).length;
  const dragged = connections.filter((c) => payload.connectionIds.includes(c.id));
  const foreignConns = dragged.filter((c) => foreign(c.vaultId)).length;
  if (foreignFolders === 0 && foreignConns === 0) return 'same';
  if (foreignFolders > 0 || payload.folderIds.length > 0 || foreignConns !== dragged.length) return 'refuse';
  return 'move';
}

/** The vault a drop onto node lands in. */
export function nodeVault(node: TreeNode): string {
  if (node.type === 'vault') return node.group?.vault ?? '';
  return node.connection?.vaultId ?? node.folder?.vaultId ?? '';
}

export function folderVault(folderId: string, folders: Folder[]): string {
  return folders.find((f) => f.id === folderId)?.vaultId ?? '';
}

/** Whether a drop on node is one that cannot happen, so no indicator promises it. */
export function refusesVaultDrop(payload: DragPayload, node: TreeNode, connections: Connection[], folders: Folder[]): boolean {
  return classifyVaultDrop(payload, nodeVault(node), connections, folders) === 'refuse';
}

/**
 * Handles a drop that crosses vaults and reports whether it did: a move goes to the confirmation
 * dialog, and a refused drop does nothing. A drop within one vault is left to the caller.
 */
export function redirectAcrossVaults(
  payload: DragPayload,
  targetVault: string,
  targetFolder: string,
  connections: Connection[],
  folders: Folder[]
): boolean {
  const kind = classifyVaultDrop(payload, targetVault, connections, folders);
  if (kind === 'move') requestVaultMove({ connectionIds: payload.connectionIds, vault: targetVault, folder: targetFolder });
  return kind !== 'same';
}
