package vaultsync

// Domain-separation labels. Each signature, hash, AEAD additional-data and HPKE info input starts
// with exactly one of these, so bytes produced for one purpose are useless for any other. They are
// part of the wire contract: changing one is a protocol major.
const (
	LabelBundle     = "xqs-sync/v1/bundle"
	LabelVKWrap     = "xqs-sync/v1/vk-wrap"
	LabelDEKWrap    = "xqs-sync/v1/dek-wrap"
	LabelBody       = "xqs-sync/v1/body"
	LabelRecordSig  = "xqs-sync/v1/record-sig"
	LabelEnvelope   = "xqs-sync/v1/envelope"
	LabelHeadSig    = "xqs-sync/v1/head-sig"
	LabelHeadHash   = "xqs-sync/v1/head-hash"
	LabelMemberSig  = "xqs-sync/v1/member-sig"
	LabelMemberHash = "xqs-sync/v1/member-hash"
	LabelDeviceSig  = "xqs-sync/v1/device-sig"
	LabelDeviceHash = "xqs-sync/v1/device-hash"
	LabelServerTLS  = "xqs-sync/v1/server-tls"
	LabelMerkleLeaf = "xqs-sync/v1/merkle-leaf"
	LabelMerkleNode = "xqs-sync/v1/merkle-node"
)

// BodyAAD is the additional data a record body is sealed with. Binding vault, record, version,
// epoch and suite means a body the server moves anywhere else - another record, an older version,
// a relabelled epoch - fails to open instead of being accepted as that record's content.
func BodyAAD(vault, record ID, version uint64, epoch uint32, suite SuiteID) ([]byte, error) {
	return NewCanon(LabelBody).ID(vault).ID(record).U64(version).U32(epoch).U16(uint16(suite)).Encoded()
}

// DEKWrapAAD binds a wrapped data key to the record version it opens, for the same reason.
func DEKWrapAAD(vault, record ID, version uint64, epoch uint32) ([]byte, error) {
	return NewCanon(LabelDEKWrap).ID(vault).ID(record).U64(version).U32(epoch).Encoded()
}

// VKWrapInfo is the HPKE info for a vault key wrapped to one member, so a wrap cannot be replayed
// to another member, another vault or another epoch.
func VKWrapInfo(vault ID, epoch uint32, member ID) ([]byte, error) {
	return NewCanon(LabelVKWrap).ID(vault).U32(epoch).ID(member).Encoded()
}

// BundleAAD binds an account key bundle to its account and suite, so the server cannot hand one
// account's bundle to a login for another and have it open.
func BundleAAD(account ID, suite SuiteID) ([]byte, error) {
	return NewCanon(LabelBundle).ID(account).U16(uint16(suite)).Encoded()
}
