package syncwire

// Version is the wire form of vaultsync.ProtocolVersion.
type Version struct {
	Major uint16 `json:"major"`
	Minor uint16 `json:"minor"`
	Patch uint16 `json:"patch"`
}

// Hello opens every session: the client states the protocol and suite it speaks.
type Hello struct {
	ClientProtocol Version `json:"clientProtocol"`
	Suite          uint16  `json:"suite"`
}

// HelloReply carries the server's supported range and its identity signature over the TLS key,
// which is what lets the client pin the identity key rather than a certificate that renews.
type HelloReply struct {
	ServerMin         Version  `json:"serverMin"`
	ServerMax         Version  `json:"serverMax"`
	Suites            []uint16 `json:"suites"`
	ServerIdentityKey []byte   `json:"serverIdentityKey"`
	TLSKeySig         []byte   `json:"tlsKeySig"`
}

// RegisterInit starts an invite-only registration.
type RegisterInit struct {
	Invite   string `json:"invite"`
	Username string `json:"username"`
	Account  string `json:"account"`
	OPAQUE   []byte `json:"opaque"`
}

// RegisterInitReply carries the server's OPAQUE registration response.
type RegisterInitReply struct {
	OPAQUE []byte `json:"opaque"`
}

// RegisterFinish uploads the OPAQUE record, the sealed bundle, the first device log entry and the
// personal vault's genesis operation, all accepted or refused together.
//
// Invite and Username travel again because the server keeps no state between the two steps: the
// invite is consumed here, in the transaction that creates the account, and nowhere else.
type RegisterFinish struct {
	Invite        string      `json:"invite"`
	Username      string      `json:"username"`
	Account       string      `json:"account"`
	Record        []byte      `json:"record"`
	IdentityKey   []byte      `json:"identityKey"`
	EncryptionKey []byte      `json:"encryptionKey"`
	Bundle        []byte      `json:"bundle"`
	Device        DeviceEntry `json:"device"`
	Vault         MemberOp    `json:"vault"`
}

// LoginInit starts an OPAQUE login.
type LoginInit struct {
	Username string `json:"username"`
	KE1      []byte `json:"ke1"`
}

// LoginInitReply carries KE2. For an unknown username the account id is a stable fake, so the
// reply has the same shape either way.
type LoginInitReply struct {
	LoginID string `json:"loginId"`
	Account string `json:"account"`
	KE2     []byte `json:"ke2"`
}

// LoginFinish completes an OPAQUE login.
type LoginFinish struct {
	LoginID string `json:"loginId"`
	KE3     []byte `json:"ke3"`
}

// LoginFinishReply is returned only after the server verified KE3.
type LoginFinishReply struct {
	AccountToken  string `json:"accountToken"`
	Bundle        []byte `json:"bundle"`
	IdentityKey   []byte `json:"identityKey"`
	EncryptionKey []byte `json:"encryptionKey"`
}

// DeviceChallenge asks for a nonce for a known device to sign.
type DeviceChallenge struct {
	Account string `json:"account"`
	Device  string `json:"device"`
}

// DeviceChallengeReply carries the single-use nonce.
type DeviceChallengeReply struct {
	Nonce []byte `json:"nonce"`
}

// DeviceVerify returns the signed nonce.
type DeviceVerify struct {
	Account string `json:"account"`
	Device  string `json:"device"`
	Nonce   []byte `json:"nonce"`
	Sig     []byte `json:"sig"`
}

// AccessReply carries a short-lived access token.
type AccessReply struct {
	AccessToken      string `json:"accessToken"`
	ExpiresUnixMilli uint64 `json:"expiresUnixMilli"`
}

// DeviceLog is a slice of an account's device log.
type DeviceLog struct {
	Entries []DeviceEntry `json:"entries"`
}

// DeviceLogAppend appends entries atomically.
type DeviceLogAppend struct {
	Entries []DeviceEntry `json:"entries"`
}

// Lookup gives an inviting admin the public keys to wrap a vault key to and to compare by
// fingerprint out of band.
type Lookup struct {
	Account       string `json:"account"`
	IdentityKey   []byte `json:"identityKey"`
	EncryptionKey []byte `json:"encryptionKey"`
	DeviceLogSeq  uint64 `json:"deviceLogSeq"`
}

// VaultSummary is one vault the caller belongs to.
type VaultSummary struct {
	Vault         string `json:"vault"`
	Seq           uint64 `json:"seq"`
	MembershipSeq uint64 `json:"membershipSeq"`
}

// VaultList tells a device which vaults it belongs to and how far each has advanced.
type VaultList struct {
	Vaults []VaultSummary `json:"vaults"`
}

// VaultCreate creates a vault with its genesis membership operation.
type VaultCreate struct {
	Op MemberOp `json:"op"`
}

// Membership is a slice of a vault's membership log.
type Membership struct {
	Ops []MemberOp `json:"ops"`
}

// MembershipAppend appends operations atomically.
type MembershipAppend struct {
	Ops []MemberOp `json:"ops"`
}

// Commit pushes one commit on top of ExpectSeq.
type Commit struct {
	ExpectSeq uint64     `json:"expectSeq"`
	Records   []Envelope `json:"records"`
	Head      Head       `json:"head"`
}

// CommitReply is the seq the commit was stored at.
type CommitReply struct {
	Seq uint64 `json:"seq"`
}

// ConflictReply accompanies a 409: the caller must pull and rebase onto CurrentSeq.
type ConflictReply struct {
	Error      string `json:"error"`
	CurrentSeq uint64 `json:"currentSeq"`
}

// CommitEntry is one commit as the server stored it, with the seq it was accepted at.
type CommitEntry struct {
	Seq     uint64     `json:"seq"`
	Head    Head       `json:"head"`
	Records []Envelope `json:"records"`
}

// Changes is a page of commits after a seq.
type Changes struct {
	Commits []CommitEntry `json:"commits"`
	More    bool          `json:"more"`
}

// Snapshot is the latest version of every record and the head that commits to them.
type Snapshot struct {
	Head    *Head      `json:"head,omitempty"`
	Records []Envelope `json:"records"`
}

// Event is a notification frame: a vault advanced, or an account's device log grew.
type Event struct {
	Vault        string `json:"vault,omitempty"`
	Seq          uint64 `json:"seq,omitempty"`
	Account      string `json:"account,omitempty"`
	DeviceLogSeq uint64 `json:"deviceLogSeq,omitempty"`
}
