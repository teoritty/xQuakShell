package usecase

import (
	"context"
	"slices"

	"xquakshell/internal/domain/vaultsync"
)

// TeamInfo is one team vault the account belongs to, as the Teams page lists it.
type TeamInfo struct {
	Vault vaultsync.ID
	Name  string
	Role  vaultsync.Role
}

// Teams lists the team vaults the account belongs to, with each one's name and this account's role.
//
// The list is read every time the Sync page reloads, which is after every round that changed
// something. Verifying each team against the server for it - its device logs, membership and name,
// twice over - made one reload cost tens of requests and tripped the server's rate limit for a
// member of a handful of teams. A team's name and the role in it change only with a commit or a
// membership op, both of which land in the state store, so a team is verified again only when its
// stored head or membership moved; the vaults are the last pass's, which the scheduler already
// fetched.
func (c *SyncController) Teams(ctx context.Context) ([]TeamInfo, error) {
	var out []TeamInfo
	err := c.withEngine(func(e *SyncEngine, s SyncSession) error {
		vaults, err := c.teamVaults(ctx, e)
		if err != nil {
			return err
		}
		for _, v := range vaults {
			if v == s.Personal {
				continue
			}
			info, err := c.cachedTeamInfo(ctx, e, v)
			if err != nil {
				return err
			}
			out = append(out, info)
		}
		return nil
	})
	return out, err
}

// teamVaults are the vaults of the scheduler's last pass, or the server's list before there was one.
func (c *SyncController) teamVaults(ctx context.Context, e *SyncEngine) ([]vaultsync.ID, error) {
	c.mu.Lock()
	known := slices.Clone(c.knownVaults)
	c.mu.Unlock()
	if known != nil {
		return known, nil
	}
	return e.Vaults(ctx)
}

// teamStamp is what a team's listing depends on, read from the state store without the network.
type teamStamp struct {
	seq uint64
	ops int
}

type cachedTeam struct {
	stamp teamStamp
	info  TeamInfo
}

func (c *SyncController) cachedTeamInfo(ctx context.Context, e *SyncEngine, v vaultsync.ID) (TeamInfo, error) {
	stamp, err := e.teamStamp(ctx, v)
	if err != nil {
		return TeamInfo{}, err
	}
	c.mu.Lock()
	hit, ok := c.teams[v]
	c.mu.Unlock()
	if ok && hit.stamp == stamp {
		return hit.info, nil
	}
	info, err := e.teamInfo(ctx, v)
	if err != nil {
		return TeamInfo{}, err
	}
	c.mu.Lock()
	if c.teams == nil {
		c.teams = map[vaultsync.ID]cachedTeam{}
	}
	c.teams[v] = cachedTeam{stamp: stamp, info: info}
	c.mu.Unlock()
	return info, nil
}

func (e *SyncEngine) teamStamp(ctx context.Context, v vaultsync.ID) (teamStamp, error) {
	stored, err := e.d.State.LoadVault(ctx, v)
	if err != nil {
		return teamStamp{}, err
	}
	ops, err := e.d.State.Membership(ctx, v)
	if err != nil {
		return teamStamp{}, err
	}
	stamp := teamStamp{ops: len(ops)}
	if stored.Head != nil {
		stamp.seq = stored.Head.Seq
	}
	return stamp, nil
}

func (e *SyncEngine) teamInfo(ctx context.Context, vault vaultsync.ID) (TeamInfo, error) {
	p, m, err := withMembers(e.startPull(ctx, vault))
	if err != nil {
		return TeamInfo{}, err
	}
	name, err := p.teamName()
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
		c.noteVault(id, true)
		c.Notify()
	}
	return id, err
}

// noteVault adds or removes a vault this device created or left in the last pass's list, so the
// team list shows the change before the next pass confirms it. A list not taken yet stays untaken:
// Teams then asks the server.
func (c *SyncController) noteVault(id vaultsync.ID, present bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.knownVaults == nil {
		return
	}
	c.knownVaults = slices.DeleteFunc(c.knownVaults, func(v vaultsync.ID) bool { return v == id })
	if present {
		c.knownVaults = append(c.knownVaults, id)
	}
	delete(c.teams, id)
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
	err := c.withEngine(func(e *SyncEngine, _ SyncSession) error { return e.LeaveTeam(ctx, vault) })
	if err == nil {
		c.noteVault(vault, false)
	}
	return err
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
