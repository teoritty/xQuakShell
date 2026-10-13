package usecase

import (
	"context"

	"xquakshell/internal/domain/vaultsync"
)

// TeamInfo is one team vault the account belongs to, as the Teams page lists it.
type TeamInfo struct {
	Vault vaultsync.ID
	Name  string
	Role  vaultsync.Role
}

// Teams lists the team vaults the account belongs to, with each one's name and this account's role.
func (c *SyncController) Teams(ctx context.Context) ([]TeamInfo, error) {
	var out []TeamInfo
	err := c.withEngine(func(e *SyncEngine, s SyncSession) error {
		vaults, err := e.Vaults(ctx)
		if err != nil {
			return err
		}
		for _, v := range vaults {
			if v == s.Personal {
				continue
			}
			info, err := e.teamInfo(ctx, v)
			if err != nil {
				return err
			}
			out = append(out, info)
		}
		return nil
	})
	return out, err
}

func (e *SyncEngine) teamInfo(ctx context.Context, vault vaultsync.ID) (TeamInfo, error) {
	_, m, err := withMembers(e.startPull(ctx, vault))
	if err != nil {
		return TeamInfo{}, err
	}
	name, err := e.TeamName(ctx, vault)
	return TeamInfo{Vault: vault, Name: name, Role: m.Role(e.d.Keys.Account())}, err
}

// CreateTeam creates a team vault owned by this account and schedules a round to sync it.
func (c *SyncController) CreateTeam(ctx context.Context, name string) (vaultsync.ID, error) {
	var id vaultsync.ID
	err := c.withEngine(func(e *SyncEngine, _ SyncSession) error {
		var err error
		id, err = e.CreateTeam(ctx, name, c.d.Random)
		return err
	})
	if err == nil {
		c.Notify()
	}
	return id, err
}

// LookupInvitee finds an account to invite and the fingerprint to compare.
func (c *SyncController) LookupInvitee(ctx context.Context, username string) (Invitee, error) {
	var inv Invitee
	err := c.withEngine(func(e *SyncEngine, _ SyncSession) error {
		var err error
		inv, err = e.LookupInvitee(ctx, username)
		return err
	})
	return inv, err
}

// Invite adds an account to a team vault.
func (c *SyncController) Invite(ctx context.Context, req InviteRequest) error {
	return c.withEngine(func(e *SyncEngine, _ SyncSession) error { return e.Invite(ctx, req) })
}

// TeamMembers lists a team vault's members.
func (c *SyncController) TeamMembers(ctx context.Context, vault vaultsync.ID) ([]MemberInfo, error) {
	var out []MemberInfo
	err := c.withEngine(func(e *SyncEngine, _ SyncSession) error {
		var err error
		out, err = e.Members(ctx, vault)
		return err
	})
	return out, err
}

// ChangeTeamRole sets a member's role.
func (c *SyncController) ChangeTeamRole(ctx context.Context, vault, account vaultsync.ID, role vaultsync.Role) error {
	return c.withEngine(func(e *SyncEngine, _ SyncSession) error { return e.ChangeRole(ctx, vault, account, role) })
}

// RemoveTeamMember removes a member and rotates the team's key.
func (c *SyncController) RemoveTeamMember(ctx context.Context, vault, account vaultsync.ID) error {
	return c.withEngine(func(e *SyncEngine, _ SyncSession) error { return e.RemoveMember(ctx, vault, account, c.d.Random) })
}

// LeaveTeam removes this account from a team vault and drops its local copy.
func (c *SyncController) LeaveTeam(ctx context.Context, vault vaultsync.ID) error {
	return c.withEngine(func(e *SyncEngine, _ SyncSession) error { return e.LeaveTeam(ctx, vault) })
}

// VerifyTeamMember records that this user compared a member's fingerprint.
func (c *SyncController) VerifyTeamMember(ctx context.Context, vault, account vaultsync.ID, fingerprint string) error {
	return c.withEngine(func(e *SyncEngine, _ SyncSession) error { return e.VerifyMember(ctx, vault, account, fingerprint) })
}

// SetTeamPolicy sets a team's retention window and whether it admits unverified members.
func (c *SyncController) SetTeamPolicy(ctx context.Context, vault vaultsync.ID, retentionDays uint16, requireVerified bool) error {
	return c.withEngine(func(e *SyncEngine, _ SyncSession) error {
		return e.SetTeamPolicy(ctx, vault, retentionDays, requireVerified)
	})
}

// MoveConnectionToTeam moves a personal connection into a team and schedules a round to push it.
func (c *SyncController) MoveConnectionToTeam(ctx context.Context, connection string, vault vaultsync.ID) error {
	err := c.withEngine(func(e *SyncEngine, _ SyncSession) error { return e.MoveConnectionToTeam(ctx, connection, vault) })
	if err == nil {
		c.Notify()
	}
	return err
}
