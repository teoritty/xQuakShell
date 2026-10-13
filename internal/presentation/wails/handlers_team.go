package wails

import (
	"context"
	"encoding/hex"
	"strings"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/usecase"
)

// TeamCommands is the slice of the sync controller the Teams page needs.
type TeamCommands interface {
	Teams(ctx context.Context) ([]usecase.TeamInfo, error)
	CreateTeam(ctx context.Context, name string) (vaultsync.ID, error)
	LookupInvitee(ctx context.Context, username string) (usecase.Invitee, error)
	Invite(ctx context.Context, req usecase.InviteRequest) error
	TeamMembers(ctx context.Context, vault vaultsync.ID) ([]usecase.MemberInfo, error)
	ChangeTeamRole(ctx context.Context, vault, account vaultsync.ID, role vaultsync.Role) error
	RemoveTeamMember(ctx context.Context, vault, account vaultsync.ID) error
	LeaveTeam(ctx context.Context, vault vaultsync.ID) error
	VerifyTeamMember(ctx context.Context, vault, account vaultsync.ID, fingerprint string) error
	SetTeamPolicy(ctx context.Context, vault vaultsync.ID, retentionDays uint16, requireVerified bool) error
	MoveConnectionToTeam(ctx context.Context, connection string, vault vaultsync.ID) error
}

// BindingCommands manages this member's credential bindings to team connections.
type BindingCommands interface {
	Bindings() ([]domain.CredentialBinding, error)
	Bind(ctx context.Context, b domain.CredentialBinding) error
	Confirm(ctx context.Context, vault, connection, user string) error
}

// SetTeamServices wires the Teams page. A setter for the reason SetLocalTerminalService gives.
func (a *AppAPI) SetTeamServices(teams TeamCommands, bindings BindingCommands) {
	a.teams, a.bindings = teams, bindings
}

// TeamDTO is one team vault the account belongs to.
type TeamDTO struct {
	Vault string `json:"vault"`
	Name  string `json:"name"`
	Role  string `json:"role"`
}

// TeamMemberDTO describes a person in a team as the members page shows them, fingerprint included.
type TeamMemberDTO struct {
	Account     string `json:"account"`
	Username    string `json:"username"`
	Fingerprint string `json:"fingerprint"`
	Role        string `json:"role"`
	Verified    bool   `json:"verified"`
	Current     bool   `json:"current"`
}

// TeamInviteeDTO is an account found to invite, with the fingerprint to compare.
type TeamInviteeDTO struct {
	Username    string `json:"username"`
	Fingerprint string `json:"fingerprint"`
	Known       bool   `json:"known"`
	Verified    bool   `json:"verified"`
}

// TeamInviteRequest invites a looked-up account; Fingerprint is the one the admin was shown.
type TeamInviteRequest struct {
	Vault       string `json:"vault"`
	Username    string `json:"username"`
	Fingerprint string `json:"fingerprint"`
	Role        string `json:"role"`
	Verified    bool   `json:"verified"`
}

// CredentialBindingDTO is one binding of a team connection user to the member's own credential.
type CredentialBindingDTO struct {
	Vault       string `json:"vault"`
	Connection  string `json:"connection"`
	User        string `json:"user"`
	Identity    string `json:"identity,omitempty"`
	Password    string `json:"password,omitempty"`
	SuspendedBy string `json:"suspendedBy,omitempty"`
}

var roleNames = map[vaultsync.Role]string{vaultsync.RoleOwner: "owner", vaultsync.RoleAdmin: "admin",
	vaultsync.RoleEditor: "editor", vaultsync.RoleViewer: "viewer"}

// parseAssignableRole reads a role an admin may give; owner is transferred, never assigned.
func parseAssignableRole(s string) (vaultsync.Role, bool) {
	for r, name := range roleNames {
		if name == s && r != vaultsync.RoleOwner {
			return r, true
		}
	}
	return vaultsync.RoleNone, false
}

// ListTeams lists the team vaults the account belongs to.
func (a *AppAPI) ListTeams() ([]TeamDTO, error) {
	if a.teams == nil {
		return nil, errSyncUnavailable
	}
	teams, err := a.teams.Teams(a.reqCtx())
	if err != nil {
		return nil, userSyncError(err)
	}
	out := make([]TeamDTO, 0, len(teams))
	for _, t := range teams {
		out = append(out, TeamDTO{Vault: hex.EncodeToString(t.Vault[:]), Name: t.Name, Role: roleNames[t.Role]})
	}
	return out, nil
}

// CreateTeam creates a team vault and returns its id.
func (a *AppAPI) CreateTeam(name string) (string, error) {
	if a.teams == nil {
		return "", errSyncUnavailable
	}
	if !validSyncField(name) || len(name) > 128 {
		return "", errInvalidSyncRequest
	}
	id, err := a.teams.CreateTeam(a.reqCtx(), strings.TrimSpace(name))
	if err != nil {
		return "", userSyncError(err)
	}
	return hex.EncodeToString(id[:]), nil
}

// LookupTeamInvitee finds an account to invite.
func (a *AppAPI) LookupTeamInvitee(username string) (TeamInviteeDTO, error) {
	if a.teams == nil {
		return TeamInviteeDTO{}, errSyncUnavailable
	}
	if !validSyncField(username) {
		return TeamInviteeDTO{}, errInvalidSyncRequest
	}
	inv, err := a.teams.LookupInvitee(a.reqCtx(), username)
	if err != nil {
		return TeamInviteeDTO{}, userSyncError(err)
	}
	return TeamInviteeDTO{Username: inv.Username, Fingerprint: inv.Fingerprint, Known: inv.Known, Verified: inv.Verified}, nil
}

// InviteToTeam adds a looked-up account to a team vault.
func (a *AppAPI) InviteToTeam(req TeamInviteRequest) error {
	if a.teams == nil {
		return errSyncUnavailable
	}
	vault, ok := parseSyncID(req.Vault)
	role, roleOK := parseAssignableRole(req.Role)
	if !ok || !roleOK || !validSyncField(req.Username) || !validSyncField(req.Fingerprint) {
		return errInvalidSyncRequest
	}
	return userSyncError(a.teams.Invite(a.reqCtx(), usecase.InviteRequest{Vault: vault, Username: req.Username,
		Fingerprint: req.Fingerprint, Role: role, Verified: req.Verified}))
}

// ListTeamMembers returns who is in a team, with roles and fingerprints to compare.
func (a *AppAPI) ListTeamMembers(vault string) ([]TeamMemberDTO, error) {
	if a.teams == nil {
		return nil, errSyncUnavailable
	}
	id, ok := parseSyncID(vault)
	if !ok {
		return nil, errInvalidSyncRequest
	}
	members, err := a.teams.TeamMembers(a.reqCtx(), id)
	if err != nil {
		return nil, userSyncError(err)
	}
	out := make([]TeamMemberDTO, 0, len(members))
	for _, m := range members {
		out = append(out, TeamMemberDTO{Account: hex.EncodeToString(m.Account[:]), Username: m.Username, Fingerprint: m.Fingerprint,
			Role: roleNames[m.Role], Verified: m.Verified, Current: m.Current})
	}
	return out, nil
}

// ChangeTeamRole sets a member's role.
func (a *AppAPI) ChangeTeamRole(vault, account, role string) error {
	if a.teams == nil {
		return errSyncUnavailable
	}
	v, okV := parseSyncID(vault)
	acct, okA := parseSyncID(account)
	r, okR := parseAssignableRole(role)
	if !okV || !okA || !okR {
		return errInvalidSyncRequest
	}
	return userSyncError(a.teams.ChangeTeamRole(a.reqCtx(), v, acct, r))
}

// RemoveTeamMember removes a member and rotates the team's key.
func (a *AppAPI) RemoveTeamMember(vault, account string) error {
	if a.teams == nil {
		return errSyncUnavailable
	}
	v, okV := parseSyncID(vault)
	acct, okA := parseSyncID(account)
	if !okV || !okA {
		return errInvalidSyncRequest
	}
	return userSyncError(a.teams.RemoveTeamMember(a.reqCtx(), v, acct))
}

// LeaveTeam removes this account from a team vault.
func (a *AppAPI) LeaveTeam(vault string) error {
	if a.teams == nil {
		return errSyncUnavailable
	}
	v, ok := parseSyncID(vault)
	if !ok {
		return errInvalidSyncRequest
	}
	return userSyncError(a.teams.LeaveTeam(a.reqCtx(), v))
}

// VerifyTeamMember records that this user compared a member's fingerprint out of band.
func (a *AppAPI) VerifyTeamMember(vault, account, fingerprint string) error {
	if a.teams == nil {
		return errSyncUnavailable
	}
	v, okV := parseSyncID(vault)
	acct, okA := parseSyncID(account)
	if !okV || !okA || !validSyncField(fingerprint) {
		return errInvalidSyncRequest
	}
	return userSyncError(a.teams.VerifyTeamMember(a.reqCtx(), v, acct, fingerprint))
}

// SetTeamPolicy sets a team's retention window (0-365 days) and whether it admits unverified members.
func (a *AppAPI) SetTeamPolicy(vault string, retentionDays int, requireVerified bool) error {
	if a.teams == nil {
		return errSyncUnavailable
	}
	v, ok := parseSyncID(vault)
	if !ok || retentionDays < 0 || retentionDays > int(vaultsync.MaxRetentionDays) {
		return errInvalidSyncRequest
	}
	return userSyncError(a.teams.SetTeamPolicy(a.reqCtx(), v, uint16(retentionDays), requireVerified)) // #nosec G115 -- bounded above
}

// MoveConnectionToTeam moves a personal connection into a team; its credentials stay personal.
func (a *AppAPI) MoveConnectionToTeam(connection, vault string) error {
	if a.teams == nil {
		return errSyncUnavailable
	}
	v, ok := parseSyncID(vault)
	if !ok || !validSyncField(connection) {
		return errInvalidSyncRequest
	}
	return userSyncError(a.teams.MoveConnectionToTeam(a.reqCtx(), connection, v))
}

// ListCredentialBindings lists this member's bindings to team connections.
func (a *AppAPI) ListCredentialBindings() ([]CredentialBindingDTO, error) {
	if a.bindings == nil {
		return nil, errSyncUnavailable
	}
	list, err := a.bindings.Bindings()
	if err != nil {
		return nil, userSyncError(err)
	}
	out := make([]CredentialBindingDTO, 0, len(list))
	for _, b := range list {
		out = append(out, CredentialBindingDTO{Vault: b.VaultID, Connection: b.ConnectionID, User: b.UserID, Identity: b.IdentityID,
			Password: b.PasswordID, SuspendedBy: b.SuspendedBy})
	}
	return out, nil
}

// BindCredential binds a team connection user to one of the member's own keys or passwords.
func (a *AppAPI) BindCredential(dto CredentialBindingDTO) error {
	if a.bindings == nil {
		return errSyncUnavailable
	}
	if _, ok := parseSyncID(dto.Vault); !ok || !validSyncField(dto.Connection) || !validSyncField(dto.User) {
		return errInvalidSyncRequest
	}
	return userSyncError(a.bindings.Bind(a.reqCtx(), domain.CredentialBinding{VaultID: dto.Vault, ConnectionID: dto.Connection,
		UserID: dto.User, IdentityID: dto.Identity, PasswordID: dto.Password}))
}

// ConfirmCredentialBinding resumes a binding another member's repointing suspended.
func (a *AppAPI) ConfirmCredentialBinding(vault, connection, user string) error {
	if a.bindings == nil {
		return errSyncUnavailable
	}
	if _, ok := parseSyncID(vault); !ok || !validSyncField(connection) || !validSyncField(user) {
		return errInvalidSyncRequest
	}
	return userSyncError(a.bindings.Confirm(a.reqCtx(), vault, connection, user))
}
