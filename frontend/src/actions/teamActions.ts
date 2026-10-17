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
  type TeamMember,
} from '../api/team';
import { errorMessage, refreshTeams } from './syncActions';
import { codeFingerprint, parseInviteInput } from '../lib/settings/sync/contactCode';
import { sameFingerprint } from '../lib/settings/sync/syncLink';
import { translate } from '../i18n/messages';
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

/**
 * Looks up a username or a contact code. With a code, matched says the key the server answered
 * with is the one its owner sent; a code that does not match returns no invitee, so the account the
 * server named instead cannot be invited from this lookup.
 */
export async function lookupInvitee(input: string): Promise<{ invitee?: TeamInvitee; matched?: boolean; error?: string }> {
  const parsed = parseInviteInput(input);
  if (!parsed) return { error: translate('sync.teams.code.malformed') };
  let invitee: TeamInvitee | null;
  try {
    invitee = await lookupTeamInviteeRpc(parsed.username);
  } catch (e) {
    return { error: errorMessage(e) };
  }
  if (!invitee) return { error: 'Sync is unavailable.' };
  if (!parsed.fingerprint) return { invitee };
  if (!sameFingerprint(parsed.fingerprint, invitee.fingerprint)) return { error: translate('security.sync.teams.code.mismatch') };
  return { invitee, matched: true };
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

/**
 * Verifies a member against the contact code or fingerprint they sent. The check here only spares
 * a round trip and names the problem; the backend compares the same fingerprint with the member's
 * genesis key and is the one that decides.
 */
export async function verifyMemberByCode(vault: string, member: Pick<TeamMember, 'account' | 'fingerprint'>, code: string): Promise<string> {
  const fingerprint = codeFingerprint(code);
  if (!fingerprint) return translate('sync.teams.code.malformed');
  if (!sameFingerprint(fingerprint, member.fingerprint)) return translate('security.sync.teams.code.mismatch');
  return verifyMember(vault, member.account, fingerprint);
}
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
