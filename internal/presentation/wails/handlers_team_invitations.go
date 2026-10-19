package wails

import "encoding/hex"

// TeamInvitationDTO is a team this account was added to and has not joined, with who added it.
type TeamInvitationDTO struct {
	Vault              string `json:"vault"`
	Name               string `json:"name"`
	Role               string `json:"role"`
	Inviter            string `json:"inviter"`
	InviterFingerprint string `json:"inviterFingerprint"`
	InviterUsername    string `json:"inviterUsername"`
	InviterKnown       bool   `json:"inviterKnown"`
	InviterVerified    bool   `json:"inviterVerified"`
}

// ListTeamInvitations lists the teams waiting for this person to accept or decline.
func (a *AppAPI) ListTeamInvitations() ([]TeamInvitationDTO, error) {
	if a.teams == nil {
		return nil, errSyncUnavailable
	}
	invs, err := a.teams.Invitations(a.reqCtx())
	if err != nil {
		return nil, userSyncError(err)
	}
	out := make([]TeamInvitationDTO, 0, len(invs))
	for _, i := range invs {
		out = append(out, TeamInvitationDTO{Vault: hex.EncodeToString(i.Vault[:]), Name: i.Name, Role: roleNames[i.Role],
			Inviter: hex.EncodeToString(i.Inviter[:]), InviterFingerprint: i.InviterFingerprint, InviterUsername: i.InviterUsername,
			InviterKnown: i.InviterKnown, InviterVerified: i.InviterVerified})
	}
	return out, nil
}

// AcceptTeamInvitation joins a team this account was invited to.
func (a *AppAPI) AcceptTeamInvitation(vault string) error {
	if a.teams == nil {
		return errSyncUnavailable
	}
	v, ok := parseSyncID(vault)
	if !ok {
		return errInvalidSyncRequest
	}
	return userSyncError(a.teams.AcceptTeam(a.reqCtx(), v))
}

// DeclineTeamInvitation answers an invitation by leaving the team.
func (a *AppAPI) DeclineTeamInvitation(vault string) error {
	if a.teams == nil {
		return errSyncUnavailable
	}
	v, ok := parseSyncID(vault)
	if !ok {
		return errInvalidSyncRequest
	}
	return userSyncError(a.teams.DeclineTeam(a.reqCtx(), v))
}
