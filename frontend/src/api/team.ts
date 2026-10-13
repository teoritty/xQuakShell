// Team vault RPC wrappers (ADR-023), one per backend method. Errors are rethrown for the forms to
// show next to the field the person has to change.
import { callBackend } from '../backend/callBackend';
import type { CredentialBinding, Team, TeamInvitee, TeamInviteRequest, TeamMember } from '../backend/teamGateway';

export type { CredentialBinding, Team, TeamInvitee, TeamInviteRequest, TeamMember };

const inline = { rethrow: true, silence: () => true };

export function listTeamsRpc(): Promise<Team[]> {
  return callBackend<Team[]>('Teams', [], (app) => app.ListTeams(), inline);
}

export function createTeamRpc(name: string): Promise<string> {
  return callBackend('Create team', '', (app) => app.CreateTeam(name), inline);
}

export function lookupTeamInviteeRpc(username: string): Promise<TeamInvitee | null> {
  return callBackend<TeamInvitee | null>('Look up account', null, (app) => app.LookupTeamInvitee(username), inline);
}

export function inviteToTeamRpc(req: TeamInviteRequest): Promise<void> {
  return callBackend('Invite member', undefined, (app) => app.InviteToTeam(req), inline);
}

export function listTeamMembersRpc(vault: string): Promise<TeamMember[]> {
  return callBackend<TeamMember[]>('Team members', [], (app) => app.ListTeamMembers(vault), inline);
}

export function changeTeamRoleRpc(vault: string, account: string, role: string): Promise<void> {
  return callBackend('Change role', undefined, (app) => app.ChangeTeamRole(vault, account, role), inline);
}

export function removeTeamMemberRpc(vault: string, account: string): Promise<void> {
  return callBackend('Remove member', undefined, (app) => app.RemoveTeamMember(vault, account), inline);
}

export function leaveTeamRpc(vault: string): Promise<void> {
  return callBackend('Leave team', undefined, (app) => app.LeaveTeam(vault), inline);
}

export function verifyTeamMemberRpc(vault: string, account: string, fingerprint: string): Promise<void> {
  return callBackend('Verify member', undefined, (app) => app.VerifyTeamMember(vault, account, fingerprint), inline);
}

export function setTeamPolicyRpc(vault: string, retentionDays: number, requireVerified: boolean): Promise<void> {
  return callBackend('Team policy', undefined, (app) => app.SetTeamPolicy(vault, retentionDays, requireVerified), inline);
}

export function listCredentialBindingsRpc(): Promise<CredentialBinding[]> {
  return callBackend<CredentialBinding[]>('Credential bindings', [], (app) => app.ListCredentialBindings(), inline);
}

export function bindCredentialRpc(binding: CredentialBinding): Promise<void> {
  return callBackend('Bind credential', undefined, (app) => app.BindCredential(binding), inline);
}

export function confirmCredentialBindingRpc(vault: string, connection: string, user: string): Promise<void> {
  return callBackend('Confirm address', undefined, (app) => app.ConfirmCredentialBinding(vault, connection, user), inline);
}

export function moveConnectionToTeamRpc(connection: string, vault: string): Promise<void> {
  return callBackend('Move to team', undefined, (app) => app.MoveConnectionToTeam(connection, vault), inline);
}
