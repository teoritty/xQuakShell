// Package vaultsync holds the pure rules of the sync protocol (ADR-023) that the application and
// the sync server must apply identically: identifiers, the canonical encoding signatures are
// computed over, suite constants, padding, the record envelope, the Merkle root, the vault head
// chain, the device and membership logs, merge, and version and retention arithmetic.
//
// It is a leaf package. It imports the standard library and nothing else from this module, so
// internal/domain can carry its types without an import cycle and the server can link it without
// pulling in a single line of client code. It encrypts nothing; sealing is infrastructure.
package vaultsync

import "errors"

// ErrIntegrity is matched by every verification failure in this package. A caller that only needs
// to stop syncing a vault and raise the alarm asks errors.Is(err, ErrIntegrity); a caller that
// reports which check failed compares against the specific error.
var ErrIntegrity = errors.New("sync integrity violation")

// integrityError is a specific verification failure that also matches ErrIntegrity.
type integrityError string

func (e integrityError) Error() string { return string(e) }

// Is makes each specific failure match the umbrella ErrIntegrity as well as itself.
func (e integrityError) Is(target error) bool { return target == ErrIntegrity }

// Local errors: produced by bad input from this device, not evidence of tampering.
var (
	ErrMalformedID         = errors.New("malformed sync identifier")
	ErrZeroID              = errors.New("sync identifier is all zero")
	ErrUnsupportedSuite    = errors.New("unsupported sync crypto suite")
	ErrSuiteBelowMinimum   = errors.New("sync crypto suite below the accepted minimum")
	ErrFieldTooLarge       = errors.New("canonical field exceeds 4 GiB")
	ErrPlaintextTooLarge   = errors.New("record plaintext exceeds the size limit")
	ErrProtocolUnsupported = errors.New("no protocol version both sides support")
)

// Integrity errors: what a client sees when data from the server or another device fails a check.
var (
	ErrMalformedEnvelope   error = integrityError("malformed record envelope")
	ErrBadPadding          error = integrityError("malformed record padding")
	ErrBadSignature        error = integrityError("signature verification failed")
	ErrUnsupportedSigAlg   error = integrityError("unsupported signature algorithm")
	ErrDuplicateLeaf       error = integrityError("record appears twice in one vault state")
	ErrChainBroken         error = integrityError("vault head does not extend the verified chain")
	ErrMembershipRegressed error = integrityError("vault head refers to an older membership state")
	ErrRollback            error = integrityError("server presented an older vault head")
	ErrFork                error = integrityError("server presented a different head at a verified position")
	ErrDeviceLog           error = integrityError("device log entry rejected")
	ErrSupersededKey       error = integrityError("signed by a superseded account identity key")
	ErrSuccessionRequired  error = integrityError("a device revocation must be followed by a key succession")
	ErrMembership          error = integrityError("membership operation rejected")
	ErrNotAuthorized       error = integrityError("author lacks the role this operation needs")
	ErrRevokedDevice       error = integrityError("author device was revoked")
	ErrRotationRequired    error = integrityError("a member removal must be followed by an epoch rotation")
	ErrUnverifiedMember    error = integrityError("vault policy requires a verified member")
	ErrVaultDeleted        error = integrityError("vault was deleted")
	ErrRootMismatch        error = integrityError("merkle root does not match the records")
	ErrCommitAuthor        error = integrityError("record does not belong to the commit that carries it")
	ErrRecordVersion       error = integrityError("record version does not follow the current one")
	ErrEpochMismatch       error = integrityError("commit does not use the vault's current epoch")
	ErrMembershipMismatch  error = integrityError("commit names a membership state other than the one given")
	ErrPurge               error = integrityError("purge names a record that is not a tombstone of this vault")
)
