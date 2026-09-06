package vault

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"filippo.io/age"

	"xquakshell/internal/domain"
)

// ReplicaSealer seals a replica document for a transport that is assumed hostile.
//
// It uses the same age + scrypt shape as the vault's own credential wraps, at the same work factor.
// The replication key is a secret a person carries between their machines, so it is a password in
// the sense that matters here - something typed, and therefore something worth stretching.
type ReplicaSealer struct{}

// NewReplicaSealer creates the sealer the replication use case seals through.
func NewReplicaSealer() ReplicaSealer {
	return ReplicaSealer{}
}

// Seal turns a document into the opaque bytes a transport carries.
//
// Two seals of the same document differ, because age mints a fresh file key each time. That matters
// beyond the usual reason: identical bytes would tell a server watching the endpoint that nothing
// changed between two pushes, which is a fact about the user's work it has no business learning.
func (ReplicaSealer) Seal(doc domain.ReplicaDocument, key string) ([]byte, error) {
	if key == "" {
		return nil, domain.ErrReplicaKeyRequired
	}
	plaintext, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("replica marshal: %w", err)
	}
	recipient, err := age.NewScryptRecipient(key)
	if err != nil {
		return nil, fmt.Errorf("replica recipient: %w", err)
	}
	// Set rather than inherited. age's own default happens to match today, so removing this line
	// would change nothing and no test could tell - but the constant that must govern this is the
	// vault's, not a library default that a dependency upgrade is free to move.
	recipient.SetWorkFactor(scryptWorkFactor)

	var buf bytes.Buffer
	writer, err := age.Encrypt(&buf, recipient)
	if err != nil {
		return nil, fmt.Errorf("replica seal init: %w", err)
	}
	if _, err := writer.Write(plaintext); err != nil {
		return nil, fmt.Errorf("replica seal write: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("replica seal close: %w", err)
	}
	return buf.Bytes(), nil
}

// Open reads back a sealed replica.
//
// A wrong key and bytes that are not a sealed replica come back as one error, worded identically,
// for the same reason the vault gives one answer to a wrong password and a wrong recovery key: the
// difference is the one bit that tells a guesser whether the key is close.
func (ReplicaSealer) Open(sealed []byte, key string) (domain.ReplicaDocument, error) {
	if key == "" {
		return domain.ReplicaDocument{}, domain.ErrReplicaKeyRequired
	}
	identity, err := age.NewScryptIdentity(key)
	if err != nil {
		return domain.ReplicaDocument{}, fmt.Errorf("replica identity: %w", err)
	}
	reader, err := age.Decrypt(bytes.NewReader(sealed), identity)
	if err != nil {
		return domain.ReplicaDocument{}, fmt.Errorf("replica open: %w", domain.ErrReplicaOpenFailed)
	}
	plaintext, err := io.ReadAll(reader)
	if err != nil {
		return domain.ReplicaDocument{}, fmt.Errorf("replica read: %w", domain.ErrReplicaOpenFailed)
	}
	var doc domain.ReplicaDocument
	if err := json.Unmarshal(plaintext, &doc); err != nil {
		return domain.ReplicaDocument{}, fmt.Errorf("replica decode: %w", domain.ErrReplicaOpenFailed)
	}
	return doc, nil
}
