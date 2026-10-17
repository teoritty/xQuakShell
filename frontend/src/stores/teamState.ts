// The team vaults the account belongs to, as every view that places items needs them: the tree
// groups a team's connections under it, the editor offers it as a place to keep one, and drag and
// drop moves between them. One list, refreshed with the vault, so no view asks the server for its
// own copy.
import { writable } from 'svelte/store';
import type { Team } from '../backend/teamGateway';

export const teams = writable<Team[]>([]);

/** Team groups the person folded away in the tree; every group starts open. */
export const collapsedVaults = writable<Set<string>>(new Set());

/** A move between vaults waiting for the person to confirm it. */
export interface VaultMoveRequest {
  connectionIds: string[];
  /** The destination: a team vault's id, or '' for the personal vault. */
  vault: string;
  /** A folder of the destination, or '' for its top level. */
  folder: string;
}

export const pendingVaultMove = writable<VaultMoveRequest | null>(null);

/** Asks the person to confirm a move; the dialog in the sidebar does the rest. */
export function requestVaultMove(req: VaultMoveRequest): void {
  if (req.connectionIds.length > 0) pendingVaultMove.set(req);
}

/** Whether this account's role in a team lets it add, change and move items there. */
export function canWrite(team: Team | undefined): boolean {
  return !!team && team.role !== 'viewer';
}

/** Whether the item is in a team this account may only view. A team still loading is not judged. */
export function isViewOnly(list: Team[], item: { vaultId?: string } | null | undefined): boolean {
  const team = item?.vaultId ? list.find((x) => x.vault === item.vaultId) : undefined;
  return !!team && !canWrite(team);
}
