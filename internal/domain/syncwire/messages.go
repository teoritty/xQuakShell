package syncwire

import "xquakshell/internal/domain/vaultsync"

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

// DeviceLog is a slice of an account's device log, with the genesis keys it replays from. The keys
// are untrusted until the reader matched the identity key against a fingerprint it already holds.
type DeviceLog struct {
	Entries       []DeviceEntry `json:"entries"`
	Suite         uint16        `json:"suite,omitempty"`
	IdentityKey   []byte        `json:"identityKey,omitempty"`
	EncryptionKey []byte        `json:"encryptionKey,omitempty"`
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
	// GenesisEncryptionKey and Entries let the inviter replay the log itself.
	GenesisEncryptionKey []byte        `json:"genesisEncryptionKey,omitempty"`
	Entries              []DeviceEntry `json:"entries,omitempty"`
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

// Commit pushes one commit on top of ExpectSeq. Purged lists tombstones the commit removes.
type Commit struct {
	ExpectSeq uint64     `json:"expectSeq"`
	Records   []Envelope `json:"records"`
	Purged    []string   `json:"purged,omitempty"`
	Head      Head       `json:"head"`
}

// CommitFrom converts a domain commit for the wire, on top of the seq below its head.
func CommitFrom(c *vaultsync.Commit) Commit {
	return Commit{ExpectSeq: c.Head.Seq - 1, Records: envelopesFrom(c.Records), Purged: idStrings(c.Purged), Head: HeadFrom(&c.Head)}
}

// ToDomain converts and checks every record, purged id and the head.
func (d Commit) ToDomain() (vaultsync.Commit, error) {
	return commitToDomain(d.Head, d.Records, d.Purged)
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
	Purged  []string   `json:"purged,omitempty"`
}

// CommitEntryFrom converts a stored commit for the wire.
func CommitEntryFrom(seq uint64, c *vaultsync.Commit) CommitEntry {
	return CommitEntry{Seq: seq, Head: HeadFrom(&c.Head), Records: envelopesFrom(c.Records), Purged: idStrings(c.Purged)}
}

// ToDomain converts and checks every record, purged id and the head.
func (d CommitEntry) ToDomain() (vaultsync.Commit, error) {
	return commitToDomain(d.Head, d.Records, d.Purged)
}

func commitToDomain(head Head, records []Envelope, purged []string) (vaultsync.Commit, error) {
	var c vaultsync.Commit
	var err error
	if c.Head, err = head.ToDomain(); err != nil {
		return vaultsync.Commit{}, err
	}
	if c.Purged, err = parseIDs(purged); err != nil {
		return vaultsync.Commit{}, err
	}
	for _, r := range records {
		e, err := r.ToDomain()
		if err != nil {
			return vaultsync.Commit{}, err
		}
		c.Records = append(c.Records, e)
	}
	return c, nil
}

func envelopesFrom(es []vaultsync.Envelope) []Envelope {
	out := make([]Envelope, 0, len(es))
	for i := range es {
		out = append(out, EnvelopeFrom(&es[i]))
	}
	return out
}

// Versions is every retained version of one record, oldest first, for the trash.
type Versions struct {
	Records []Envelope `json:"records"`
}

// Changes is a page of commits after a seq.
type Changes struct {
	Commits []CommitEntry `json:"commits"`
	More    bool          `json:"more"`
}

// Snapshot is one page of the latest version of every record, with the head that commits to all of
// them. Pages run in record order: the next one is asked for after the last record of this one, and
// More says there is one. Every page carries the head it was read under, so a client sees when the
// vault moved between two pages.
type Snapshot struct {
	Head    *Head      `json:"head,omitempty"`
	Records []Envelope `json:"records"`
	More    bool       `json:"more"`
}

// Heads is a page of the signed heads after a seq, in seq order, without their records: a device
// whose history the server pruned checks with them that the snapshot it starts again from extends
// the chain it verified.
type Heads struct {
	Heads []Head `json:"heads"`
}

// Event is a notification frame: a vault advanced, or an account's device log grew.
type Event struct {
	Vault         string `json:"vault,omitempty"`
	Seq           uint64 `json:"seq,omitempty"`
	MembershipSeq uint64 `json:"membershipSeq,omitempty"`
	Account       string `json:"account,omitempty"`
	DeviceLogSeq  uint64 `json:"deviceLogSeq,omitempty"`
}

// PasswordInit starts an OPAQUE re-registration for the signed-in account.
type PasswordInit struct {
	OPAQUE []byte `json:"opaque"`
}

// PasswordInitReply carries the server's registration response.
type PasswordInitReply struct {
	OPAQUE []byte `json:"opaque"`
}

// PasswordFinish replaces the record and the bundle; Sig is the account's current identity key over
// vaultsync.RekeyMessage of the pair.
type PasswordFinish struct {
	Record []byte `json:"record"`
	Bundle []byte `json:"bundle"`
	Sig    []byte `json:"sig"`
}
