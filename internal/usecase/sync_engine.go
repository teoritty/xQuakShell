package usecase

import (
	"context"
	"errors"
	"fmt"
	"net"

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
	// MembershipSeq is how far the vault's verified membership log reaches, zero when the round
	// never read it. A role change or a removal moves it without carrying a record to apply.
	MembershipSeq int
	// NewDevices are devices certified in the account since the last pull.
	NewDevices []vaultsync.DeviceCert
}

// pullPageSize is how many commits one changes request asks for.
const pullPageSize = 100

// integrity marks err as a verification failure. The wrapped cause stays inspectable for the audit
// log; the UI shows only that the vault stopped.
//
// A failure to reach the data is not a verification failure and is returned as it is. Verifying a
// commit fetches the device logs of the members who signed it, and a rate limit or a dropped
// connection during that fetch used to come back wrapped as integrity: the vault stopped for good
// and told the person to suspect the server, for a 429. Nothing is applied in either case; the
// difference is that an availability failure is retried, as it must be.
func integrity(what string, err error) error {
	if errors.Is(err, synctrust.ErrIntegrity) || unavailable(err) {
		return err
	}
	return fmt.Errorf("%w: %s: %w", synctrust.ErrIntegrity, what, err)
}

// unavailable reports a failure to reach the server or to get an answer in time, as opposed to an
// answer that is wrong. A server that refuses or contradicts is never in this list.
func unavailable(err error) bool {
	var limited *synctrust.RateLimitedError
	var netErr net.Error
	return errors.As(err, &limited) || errors.As(err, &netErr) || errors.Is(err, synctrust.ErrRemoteInternal) ||
		errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)
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
