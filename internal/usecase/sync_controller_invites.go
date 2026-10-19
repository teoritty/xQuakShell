package usecase

import (
	"context"
	"slices"

	"xquakshell/internal/domain/vaultsync"
)

// Invitations lists the teams this account was added to and has not joined, for the person to
// accept or decline. The last listing is kept, and an invitation whose membership did not move is
// not verified again.
func (c *SyncController) Invitations(ctx context.Context) ([]Invitation, error) {
	var out []Invitation
	err := c.withEngine(func(e *SyncEngine, _ SyncSession) error {
		c.mu.Lock()
		known := slices.Clone(c.invites)
		c.mu.Unlock()
		var err error
		out, err = e.Invitations(ctx, known)
		return err
	})
	if err == nil {
		c.mu.Lock()
		c.invites = slices.Clone(out)
		c.mu.Unlock()
	}
	return out, err
}

// AcceptTeam joins a team this account was invited to and schedules a round to sync it in.
func (c *SyncController) AcceptTeam(ctx context.Context, vault vaultsync.ID) error {
	err := c.withEngine(func(e *SyncEngine, _ SyncSession) error { return e.AcceptTeam(ctx, vault) })
	c.forgetInvitation(vault)
	if err == nil {
		c.noteVault(vault, true)
		c.Notify()
	}
	return err
}

// DeclineTeam answers an invitation by leaving the team.
func (c *SyncController) DeclineTeam(ctx context.Context, vault vaultsync.ID) error {
	err := c.withEngine(func(e *SyncEngine, _ SyncSession) error { return e.DeclineTeam(ctx, vault) })
	c.forgetInvitation(vault)
	return err
}

func (c *SyncController) forgetInvitation(vault vaultsync.ID) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.invites = slices.DeleteFunc(c.invites, func(i Invitation) bool { return i.Vault == vault })
}
