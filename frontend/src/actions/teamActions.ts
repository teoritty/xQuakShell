// Team orchestration: each action calls one RPC and returns the message to show next to the form -
// an empty string is success - so a component never has to know how an error is shaped.
import { get } from 'svelte/store';
import {
  bindCredentialRpc,
  changeTeamRoleRpc,
  confirmCredentialBindingRpc,
  createTeamRpc,
  inviteToTeamRpc,
  leaveTeamRpc,
  moveConnectionToVaultRpc,
  lookupTeamInviteeRpc,
  removeTeamMemberRpc,
  setTeamPolicyRpc,
  verifyTeamMemberRpc,
  type CredentialBinding,
  type TeamInvitee,
} from '../api/team';
import { errorMessage, refreshTeams } from './syncActions';
import { refreshAllConnections } from './connectionActions';
import { detailsConnectionId, favorites, selectedConnectionId, selectedConnectionIds } from '../stores/appState';

async function attempt(fn: () => Promise<unknown>): Promise<string> {
  try {
    await fn();
    return '';
  } catch (e) {
    return errorMessage(e);
  }
}

/** Creates a team; a blank name is refused here, before any RPC. */
export async function createTeam(name: string): Promise<{ vault: string; error: string }> {
  const trimmed = name.trim();
  if (trimmed === '') return { vault: '', error: 'Give the team a name.' };
  let vault = '';
  const error = await attempt(async () => {
    vault = await createTeamRpc(trimmed);
  });
  if (!error) await refreshTeams();
  return { vault, error };
}

export async function lookupInvitee(username: string): Promise<{ invitee?: TeamInvitee; error?: string }> {
  try {
    const invitee = await lookupTeamInviteeRpc(username.trim());
    return invitee ? { invitee } : { error: 'Sync is unavailable.' };
  } catch (e) {
    return { error: errorMessage(e) };
  }
}

/**
 * Invites the account a lookup returned. The fingerprint sent back is the one the admin was shown,
 * so the backend refuses the invite if the account's key changed in between.
 */
export const inviteMember = (vault: string, invitee: TeamInvitee, role: string, verified: boolean) =>
  attempt(() => inviteToTeamRpc({ vault, username: invitee.username, fingerprint: invitee.fingerprint, role, verified }));

export const changeRole = (vault: string, account: string, role: string) => attempt(() => changeTeamRoleRpc(vault, account, role));
export const removeMember = (vault: string, account: string) => attempt(() => removeTeamMemberRpc(vault, account));
export async function leaveTeam(vault: string): Promise<string> {
  const error = await attempt(() => leaveTeamRpc(vault));
  await Promise.all([refreshTeams(), refreshAllConnections()]);
  return error;
}
export const verifyMember = (vault: string, account: string, fingerprint: string) =>
  attempt(() => verifyTeamMemberRpc(vault, account, fingerprint));
export const setTeamPolicy = (vault: string, days: number, requireVerified: boolean) =>
  attempt(() => setTeamPolicyRpc(vault, days, requireVerified));
export const bindCredential = (binding: CredentialBinding) => attempt(() => bindCredentialRpc(binding));
export const confirmBinding = (b: CredentialBinding) => attempt(() => confirmCredentialBindingRpc(b.vault, b.connection, b.user));

/**
 * Moves connections into vault ('' is the personal one), at its top level or into folder. A moved
 * connection gets a new id, so whatever pointed at the old one - the selection, the open editor,
 * a favourite - is pointed at the new one; without that, moving the connection the editor shows
 * would close the editor, and a moved favourite would silently stop being one.
 *
 * Moves run one at a time and stop at the first refusal; the ones already made stay made and are
 * shown, since each is complete on its own.
 */
export async function moveConnectionsToVault(ids: string[], vault: string, folder: string): Promise<string> {
  const moved = new Map<string, string>();
  let error = '';
  for (const id of ids) {
    try {
      moved.set(id, await moveConnectionToVaultRpc(id, vault, folder));
    } catch (e) {
      error = errorMessage(e);
      break;
    }
  }
  if (moved.size > 0) {
    await refreshAllConnections();
    followMovedConnections(moved);
  }
  return error;
}

function followMovedConnections(moved: Map<string, string>): void {
  const follow = (id: string) => moved.get(id) ?? id;
  favorites.update((set) => new Set([...set].map(follow)));
  selectedConnectionIds.update((set) => new Set([...set].map(follow)));
  selectedConnectionId.update(follow);
  detailsConnectionId.update(follow);
}
