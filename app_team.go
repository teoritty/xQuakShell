package main

import "xquakshell/internal/presentation/wails"

// ListTeams lists the team vaults the account belongs to.
func (a *App) ListTeams() ([]wails.TeamDTO, error) { return a.api.ListTeams() }

// CreateTeam starts a team vault this account owns and returns its id.
func (a *App) CreateTeam(name string) (string, error) { return a.api.CreateTeam(name) }

// LookupTeamInvitee finds an account to invite and its fingerprint.
func (a *App) LookupTeamInvitee(username string) (wails.TeamInviteeDTO, error) {
	return a.api.LookupTeamInvitee(username)
}

// InviteToTeam adds a looked-up account to a team vault.
func (a *App) InviteToTeam(req wails.TeamInviteRequest) error { return a.api.InviteToTeam(req) }

// ListTeamMembers returns who is in a team, with roles and fingerprints to compare.
func (a *App) ListTeamMembers(vault string) ([]wails.TeamMemberDTO, error) {
	return a.api.ListTeamMembers(vault)
}

// ChangeTeamRole sets a member's role.
func (a *App) ChangeTeamRole(vault, account, role string) error {
	return a.api.ChangeTeamRole(vault, account, role)
}

// RemoveTeamMember removes a member and rotates the team's key.
func (a *App) RemoveTeamMember(vault, account string) error {
	return a.api.RemoveTeamMember(vault, account)
}

// LeaveTeam removes this account from a team vault.
func (a *App) LeaveTeam(vault string) error { return a.api.LeaveTeam(vault) }

// VerifyTeamMember records a fingerprint compared out of band.
func (a *App) VerifyTeamMember(vault, account, fingerprint string) error {
	return a.api.VerifyTeamMember(vault, account, fingerprint)
}

// SetTeamPolicy sets a team's retention window and verification policy.
func (a *App) SetTeamPolicy(vault string, retentionDays int, requireVerified bool) error {
	return a.api.SetTeamPolicy(vault, retentionDays, requireVerified)
}

// ListCredentialBindings lists this member's bindings to team connections.
func (a *App) ListCredentialBindings() ([]wails.CredentialBindingDTO, error) {
	return a.api.ListCredentialBindings()
}

// BindCredential binds a team connection user to one of the member's own credentials.
func (a *App) BindCredential(dto wails.CredentialBindingDTO) error { return a.api.BindCredential(dto) }

// ConfirmCredentialBinding resumes a binding another member's repointing suspended.
func (a *App) ConfirmCredentialBinding(vault, connection, user string) error {
	return a.api.ConfirmCredentialBinding(vault, connection, user)
}

// MoveConnectionToVault moves a connection between the personal vault and team vaults and returns
// its new id; its credentials stay personal.
func (a *App) MoveConnectionToVault(connection, vault, folder string) (string, error) {
	return a.api.MoveConnectionToVault(connection, vault, folder)
}

// ListTeamInvitations lists the teams waiting for this person to accept or decline.
func (a *App) ListTeamInvitations() ([]wails.TeamInvitationDTO, error) {
	return a.api.ListTeamInvitations()
}

// AcceptTeamInvitation joins a team this account was invited to.
func (a *App) AcceptTeamInvitation(vault string) error { return a.api.AcceptTeamInvitation(vault) }

// DeclineTeamInvitation answers an invitation by leaving the team.
func (a *App) DeclineTeamInvitation(vault string) error { return a.api.DeclineTeamInvitation(vault) }
