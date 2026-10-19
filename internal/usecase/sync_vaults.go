package usecase

import (
	"context"
	"errors"
	"slices"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// scopeOf places a vault in the local vault: the account's personal vault is the personal items,
// any other is the team whose id the items carry.
func (e *SyncEngine) scopeOf(vault vaultsync.ID) synctrust.Scope {
	if vault == synctrust.PersonalVaultID(e.d.Keys.Account()) {
		return synctrust.Scope{}
	}
	return synctrust.Scope{Vault: vault.String()}
}

// Vaults lists the vaults to sync: the personal vault first, then the team vaults this account
// joined. A listed team not joined is an invitation, left for the person to answer (Invitations). A
// team vault this device holds items of that the server no longer lists is checked once more: when
// its membership answers not found, the account was removed or the vault deleted, and the local copy
// of its items is dropped (spec §5.9). The server can withhold a vault - it can withhold everything -
// but nothing here lets it change what a vault holds, or add one.
func (e *SyncEngine) Vaults(ctx context.Context) ([]vaultsync.ID, error) {
	personal := synctrust.PersonalVaultID(e.d.Keys.Account())
	listed, err := e.d.Server.Vaults(ctx)
	if err != nil {
		return []vaultsync.ID{personal}, err
	}
	joined, _, err := e.joinedTeams(ctx, listed)
	if err != nil {
		return []vaultsync.ID{personal}, err
	}
	out := append([]vaultsync.ID{personal}, joined...)
	return out, e.dropLeftVaults(ctx, out)
}

func (e *SyncEngine) dropLeftVaults(ctx context.Context, keep []vaultsync.ID) error {
	data, err := e.d.Vault.GetData()
	if err != nil {
		return err
	}
	for _, id := range localTeamVaults(data) {
		if slices.Contains(keep, id) {
			continue
		}
		if _, err := e.d.Server.Membership(ctx, id, 0); !errors.Is(err, synctrust.ErrRemoteNotFound) {
			continue
		}
		if err := e.forgetTeam(ctx, id); err != nil {
			return err
		}
	}
	return nil
}

// localTeamVaults lists the team vaults the local vault holds items of.
func localTeamVaults(d *domain.VaultData) []vaultsync.ID {
	seen := map[string]bool{}
	add := func(v string) {
		if v != "" {
			seen[v] = true
		}
	}
	for _, f := range d.Folders {
		add(f.VaultID)
	}
	for _, c := range d.Connections {
		add(c.VaultID)
	}
	for _, i := range d.Identities {
		add(i.VaultID)
	}
	for _, p := range d.Passwords {
		add(p.VaultID)
	}
	var out []vaultsync.ID
	for v := range seen {
		if id, err := vaultsync.ParseID(v); err == nil {
			out = append(out, id)
		}
	}
	sortIDs(out)
	return out
}

// forgetTeam removes a team's items from the local vault and its state from state.db, and the
// record that this account joined it, so the account's other devices drop it too and a later
// invitation to it is asked about again.
func (e *SyncEngine) forgetTeam(ctx context.Context, vault vaultsync.ID) error {
	v := vault.String()
	err := e.d.Vault.UpdateData(ctx, func(d *domain.VaultData) error {
		if d.Sync != nil {
			d.Sync.Teams = slices.DeleteFunc(d.Sync.Teams, func(t domain.SyncTeam) bool { return t.Vault == v })
		}
		d.Folders = slices.DeleteFunc(d.Folders, func(f domain.ConnectionFolder) bool { return f.VaultID == v })
		d.Connections = slices.DeleteFunc(d.Connections, func(c domain.Connection) bool { return c.VaultID == v })
		for id, i := range d.Identities {
			if i.VaultID == v {
				delete(d.Identities, id)
				delete(d.KeyBlobs, id)
			}
		}
		for id, p := range d.Passwords {
			if p.VaultID == v {
				delete(d.Passwords, id)
			}
		}
		return nil
	})
	if err != nil {
		return err
	}
	return e.d.State.ForgetVault(ctx, vault)
}
