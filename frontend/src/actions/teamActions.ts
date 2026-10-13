// Team orchestration: each action calls one RPC and returns the message to show next to the form -
// an empty string is success - so a component never has to know how an error is shaped.
import {
  bindCredentialRpc,
  changeTeamRoleRpc,
  confirmCredentialBindingRpc,
  createTeamRpc,
  inviteToTeamRpc,
  leaveTeamRpc,
  moveConnectionToTeamRpc,
  lookupTeamInviteeRpc,
  removeTeamMemberRpc,
  setTeamPolicyRpc,
  verifyTeamMemberRpc,
  type CredentialBinding,
  type TeamInvitee,
} from '../api/team';
import { errorMessage } from './syncActions';

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
export const leaveTeam = (vault: string) => attempt(() => leaveTeamRpc(vault));
export const verifyMember = (vault: string, account: string, fingerprint: string) =>
  attempt(() => verifyTeamMemberRpc(vault, account, fingerprint));
export const setTeamPolicy = (vault: string, days: number, requireVerified: boolean) =>
  attempt(() => setTeamPolicyRpc(vault, days, requireVerified));
export const bindCredential = (binding: CredentialBinding) => attempt(() => bindCredentialRpc(binding));
export const confirmBinding = (b: CredentialBinding) => attempt(() => confirmCredentialBindingRpc(b.vault, b.connection, b.user));
export const moveConnectionToTeam = (connection: string, vault: string) => attempt(() => moveConnectionToTeamRpc(connection, vault));
