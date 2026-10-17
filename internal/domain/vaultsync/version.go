package vaultsync

import (
	"cmp"
	"fmt"
)

// ProtocolVersion is the syncProtocol axis (ADR-023): SemVer, additive within a major.
type ProtocolVersion struct {
	Major uint16
	Minor uint16
	Patch uint16
}

// CurrentProtocol is the version this build speaks.
var CurrentProtocol = ProtocolVersion{Major: 1}

// Compare orders versions by major, minor, then patch.
func (v ProtocolVersion) Compare(o ProtocolVersion) int {
	if c := cmp.Compare(v.Major, o.Major); c != 0 {
		return c
	}
	if c := cmp.Compare(v.Minor, o.Minor); c != 0 {
		return c
	}
	return cmp.Compare(v.Patch, o.Patch)
}

// String renders the version as major.minor.patch.
func (v ProtocolVersion) String() string { return fmt.Sprintf("%d.%d.%d", v.Major, v.Minor, v.Patch) }

// NegotiateProtocol picks the version to speak: the lower of the client's and the server's newest,
// provided it shares the client's major and is not below the server's oldest. Minors are additive,
// so a newer client can always speak an older server's minor of the same major; a major mismatch
// has no common language.
func NegotiateProtocol(client, serverMin, serverMax ProtocolVersion) (ProtocolVersion, error) {
	if serverMin.Compare(serverMax) > 0 {
		return ProtocolVersion{}, fmt.Errorf("server range %v..%v: %w", serverMin, serverMax, ErrProtocolUnsupported)
	}
	chosen := client
	if client.Compare(serverMax) > 0 {
		chosen = serverMax
	}
	if chosen.Major != client.Major || chosen.Compare(serverMin) < 0 {
		return ProtocolVersion{}, fmt.Errorf("client %v, server %v..%v: %w", client, serverMin, serverMax, ErrProtocolUnsupported)
	}
	return chosen, nil
}

// SchemaAccess is what this build may do with a record of a given type and schema.
type SchemaAccess uint8

const (
	// SchemaWritable: the type is known and the schema is not newer than this build's.
	SchemaWritable SchemaAccess = iota + 1
	// SchemaReadOnly: a newer schema. Shown, never written, because a write would drop the fields
	// this build does not know - the unknown-field preservation ADR-017 chose not to build.
	SchemaReadOnly
	// SchemaOpaque: an unknown type. Kept as received and never rewritten.
	SchemaOpaque
)

// RecordAccess decides access from the record type registry of this build.
func RecordAccess(known map[string]uint16, recordType string, schema uint16) SchemaAccess {
	current, ok := known[recordType]
	if !ok {
		return SchemaOpaque
	}
	if schema > current {
		return SchemaReadOnly
	}
	return SchemaWritable
}

// LabelHelloContext separates the hello transcript hash from every other digest.
const LabelHelloContext = "xqs-sync/v1/hello-context"

// HelloContext is the OPAQUE AKE context both sides derive from what the server offered in hello:
// its identity key, its protocol range and its suites. A network attacker who alters any of them on
// the way to the client makes the two sides derive different contexts, and the login fails instead
// of silently proceeding on a downgraded offer.
func HelloContext(identityKey []byte, serverMin, serverMax ProtocolVersion, suites []SuiteID) ([]byte, error) {
	c := NewCanon(LabelHelloContext).Bytes(identityKey).
		U16(serverMin.Major).U16(serverMin.Minor).U16(serverMin.Patch).
		U16(serverMax.Major).U16(serverMax.Minor).U16(serverMax.Patch).U64(uint64(len(suites)))
	for _, s := range suites {
		c.U16(uint16(s))
	}
	sum, err := c.Sum()
	return sum[:], err
}
