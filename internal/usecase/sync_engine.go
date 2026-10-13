package usecase

import (
	"errors"
	"fmt"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// SyncEngineDeps are the ports the sync engine drives (ADR-023).
type SyncEngineDeps struct {
	Server synctrust.SyncServer
	State  synctrust.StateStore
	Vault  domain.VaultRepository
	Keys   synctrust.KeyRing
	Cipher synctrust.RecordCipher
	// Crypto wraps vault keys; only epoch rotation and rewrap need it.
	Crypto synctrust.AccountCrypto
	// NowUnixMilli stamps quarantine entries and commit heads.
	NowUnixMilli func() int64
}

// SyncEngine moves one device's vaults in step with the sync server: it pulls, verifies and applies
// what other devices committed, and pushes what this device changed. The server is untrusted
// throughout: every commit is verified against the device log and membership before any of it
// reaches the vault, and a failed verification stops the vault rather than skipping the commit.
type SyncEngine struct {
	d SyncEngineDeps
}

// NewSyncEngine wires the engine; it starts nothing, the scheduler decides when to pull and push.
func NewSyncEngine(d SyncEngineDeps) *SyncEngine { return &SyncEngine{d: d} }

// PullReport counts what one pull did, for the status the UI shows.
type PullReport struct {
	Commits     int
	Applied     int
	Held        int
	Quarantined int
	// NewDevices are devices certified in the account since the last pull.
	NewDevices []vaultsync.DeviceCert
}

// pullPageSize is how many commits one changes request asks for.
const pullPageSize = 100

// integrity marks err as a verification failure. The wrapped cause stays inspectable for the audit
// log; the UI shows only that the vault stopped.
func integrity(what string, err error) error {
	if errors.Is(err, synctrust.ErrIntegrity) {
		return err
	}
	return fmt.Errorf("%w: %s: %w", synctrust.ErrIntegrity, what, err)
}

// syncKeyName names the record a key-derived id belongs to, learnt from its body.
type syncKeyName struct {
	ty  synctrust.RecordType
	key string
}

func indexEnvelopes(records []vaultsync.Envelope) map[vaultsync.ID]vaultsync.Envelope {
	out := make(map[vaultsync.ID]vaultsync.Envelope, len(records))
	for _, e := range records {
		out[e.Record] = e
	}
	return out
}
