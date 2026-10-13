package usecase

import (
	"context"
	"errors"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// JoinChoice is what the user picked when this device and the account both hold data (spec §7.5).
type JoinChoice int

const (
	// JoinMerge keeps both: items merge by id, and nothing on either side is lost.
	JoinMerge JoinChoice = iota + 1
	// JoinReplace replaces this device's synced data with the account's.
	JoinReplace
)

var (
	// ErrJoinChoice is a join choice that is neither merge nor replace.
	ErrJoinChoice = errors.New("sync: choose merge or replace")
	// ErrNoBackup is a join attempted without a way to back the vault up first.
	ErrNoBackup = errors.New("sync: the vault cannot be backed up, so it will not be changed")
)

// joinBackupReason names the vault copy taken before a join.
const joinBackupReason = "sync-join"

// JoinNeeded reports whether the user must choose how to join: only when this device already holds
// synced items and the account's personal vault already holds records beyond its naming key.
func (s *SyncAccountService) JoinNeeded(ctx context.Context, session SyncSession) (bool, error) {
	data, err := s.d.Vault.GetData()
	if err != nil || !hasSyncedItems(data) {
		return false, err
	}
	snap, err := session.Server.Snapshot(ctx, session.Personal)
	if err != nil || snap.Head == nil {
		return false, err
	}
	meta := synctrust.VaultMetaID(session.Personal)
	for _, r := range snap.Records {
		if r.Record != meta && !r.Deleted {
			return true, nil
		}
	}
	return false, nil
}

// Join backs the vault up and then merges this device's data with the account's, or replaces it.
// Nothing changes if the backup fails. It returns the backup's file name for the user.
func (s *SyncAccountService) Join(ctx context.Context, session SyncSession, choice JoinChoice) (string, SyncReport, error) {
	if choice != JoinMerge && choice != JoinReplace {
		return "", SyncReport{}, ErrJoinChoice
	}
	if s.d.Backup == nil {
		return "", SyncReport{}, ErrNoBackup
	}
	name, err := s.d.Backup.BackupNow(ctx, joinBackupReason)
	if err != nil {
		return "", SyncReport{}, errors.Join(ErrNoBackup, err)
	}
	engine := s.d.NewEngine(session)
	if choice == JoinMerge {
		r, err := engine.Sync(ctx, session.Personal)
		return name, r, err
	}
	if err := s.d.Vault.UpdateData(ctx, func(d *domain.VaultData) error { clearSyncedItems(d); return nil }); err != nil {
		return name, SyncReport{}, err
	}
	pulled, err := engine.AdoptRemote(ctx, session.Personal)
	if err != nil {
		return name, SyncReport{Pull: pulled}, err
	}
	r, err := engine.Sync(ctx, session.Personal)
	r.Pull = addPull(pulled, r.Pull)
	return name, r, err
}

func hasSyncedItems(d *domain.VaultData) bool {
	return len(d.Folders)+len(d.Connections)+len(d.Identities)+len(d.Passwords)+len(d.PluginSecrets)+
		len(d.KnownHosts)+len(d.PeerTrust) > 0
}

// clearSyncedItems empties everything a sync record can hold except the settings, which the
// account's sections overwrite on arrival, and the device's own sync account.
func clearSyncedItems(d *domain.VaultData) {
	d.Folders, d.Connections, d.KnownHosts, d.PeerTrust = nil, nil, nil, nil
	d.Identities, d.KeyBlobs = map[string]domain.SSHIdentity{}, map[string]domain.IdentityBlob{}
	d.Passwords, d.PluginSecrets = map[string]domain.PasswordBlob{}, map[string][]byte{}
	if d.Sync != nil {
		d.Sync.Conflicts = nil
	}
}

// AdoptRemote pulls a vault applying the account's version of every item over this device's,
// without merging - the replace choice of a join.
func (e *SyncEngine) AdoptRemote(ctx context.Context, vault vaultsync.ID) (PullReport, error) {
	p, err := e.startPull(ctx, vault)
	if err != nil {
		return PullReport{}, err
	}
	p.preferRemote = true
	err = p.pullAll(ctx)
	return p.report, err
}
