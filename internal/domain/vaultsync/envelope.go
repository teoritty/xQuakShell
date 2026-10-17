package vaultsync

import (
	"crypto/ed25519"
	"fmt"
)

// Envelope is the server-visible form of one record version (spec section 4.1). The server stores
// and orders envelopes and can read every field here; the record's type and content are inside
// Body, sealed under a data key only vault members can unwrap.
type Envelope struct {
	Vault      ID
	Record     ID
	Version    uint64
	Epoch      uint32
	Suite      SuiteID
	Deleted    bool
	WrappedDEK []byte
	Body       []byte
	Author     ID
	SigAlg     SigAlg
	Sig        []byte
}

func malformed(reason string) error {
	return fmt.Errorf("%w: %s", ErrMalformedEnvelope, reason)
}

// Validate checks structure before any signature or cipher runs, so oversized or inconsistent
// input from the server is refused at the cheapest point.
func (e *Envelope) Validate() error {
	if e.Vault.IsZero() || e.Record.IsZero() || e.Author.IsZero() {
		return malformed("zero identifier")
	}
	if e.Version == 0 {
		return malformed("version 0")
	}
	if e.SigAlg != SigEd25519 {
		return fmt.Errorf("%w: algorithm %d", ErrUnsupportedSigAlg, e.SigAlg)
	}
	if len(e.Sig) != ed25519.SignatureSize {
		return malformed("signature size")
	}
	params, err := LookupSuite(e.Suite, MinAcceptedSuite)
	if err != nil {
		return fmt.Errorf("%w: %w", ErrMalformedEnvelope, err)
	}
	return e.validateContent(params)
}

// validateContent enforces that a tombstone carries nothing and a live version carries a key of
// the suite's exact size and a body within the padding bounds.
func (e *Envelope) validateContent(p SuiteParams) error {
	if e.Deleted {
		if len(e.WrappedDEK) != 0 || len(e.Body) != 0 {
			return malformed("tombstone carries content")
		}
		return nil
	}
	if len(e.WrappedDEK) != p.WrappedDEKSize {
		return malformed("wrapped key size")
	}
	minBody := MinPaddedSize + p.AEADOverhead
	maxBody := PaddedSize(MaxPlaintextSize) + p.AEADOverhead
	if len(e.Body) < minBody || len(e.Body) > maxBody {
		return malformed("body size")
	}
	return nil
}

// SigningBytes is the canonical encoding the author device signs: every field except Sig.
func (e *Envelope) SigningBytes() ([]byte, error) {
	return NewCanon(LabelRecordSig).ID(e.Vault).ID(e.Record).U64(e.Version).U32(e.Epoch).
		U16(uint16(e.Suite)).Bool(e.Deleted).Bytes(e.WrappedDEK).Bytes(e.Body).ID(e.Author).
		U16(uint16(e.SigAlg)).Encoded()
}

// Digest commits to the whole envelope including its signature; it is what a Merkle leaf holds,
// so the vault head commits to who signed each version and not only to what it says.
func (e *Envelope) Digest() ([32]byte, error) {
	msg, err := e.SigningBytes()
	if err != nil {
		return [32]byte{}, err
	}
	return NewCanon(LabelEnvelope).Bytes(msg).Bytes(e.Sig).Sum()
}

// Verify validates structure and then the author's signature.
func (e *Envelope) Verify(publicKey []byte) error {
	if err := e.Validate(); err != nil {
		return err
	}
	msg, err := e.SigningBytes()
	if err != nil {
		return err
	}
	return VerifySignature(e.SigAlg, publicKey, msg, e.Sig)
}
