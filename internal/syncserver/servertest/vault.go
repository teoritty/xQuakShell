package servertest

import (
	"bytes"
	"crypto/ed25519"
	"slices"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

// ClientVault is a vault as its members' devices track it: the membership and record states they
// verified, used to build ops and commits the server must accept or refuse.
type ClientVault struct {
	T     *testing.T
	H     *Harness
	ID    vaultsync.ID
	M     *vaultsync.MembershipState
	State *vaultsync.VaultState
	Users map[vaultsync.ID]*User
	// OpsSoFar are the membership ops the server accepted, in order.
	OpsSoFar []vaultsync.MemberOp
}

// Resolve maps a device to its account and key, as a client would through member device logs.
func (cv *ClientVault) Resolve(device vaultsync.ID) (vaultsync.ID, []byte, error) {
	for _, u := range cv.Users {
		if u.Device == device {
			return u.Account, u.DSK.Public().(ed25519.PublicKey), nil
		}
	}
	return vaultsync.ID{}, nil, vaultsync.ErrDeviceLog
}

// Check is the membership signature check over the known users.
func (cv *ClientVault) Check(account, device vaultsync.ID, alg vaultsync.SigAlg, msg, sig []byte) error {
	acct, key, err := cv.Resolve(device)
	if err != nil || acct != account {
		return vaultsync.ErrBadSignature
	}
	return vaultsync.VerifySignature(alg, key, msg, sig)
}

// SignOp fills and signs an op at the next position of m without applying it.
func (cv *ClientVault) SignOp(u *User, m *vaultsync.MembershipState, op vaultsync.MemberOp) vaultsync.MemberOp {
	cv.T.Helper()
	op.Vault, op.Seq, op.Prev = cv.ID, m.Seq()+1, m.Hash()
	op.Author, op.AuthorDevice, op.Suite, op.SigAlg = u.Account, u.Device, vaultsync.Suite1, vaultsync.SigEd25519
	msg, err := op.SigningBytes()
	if err != nil {
		cv.T.Fatal(err)
	}
	op.Sig = ed25519.Sign(u.DSK, msg)
	return op
}

// Replay rebuilds the membership state from the accepted ops.
func (cv *ClientVault) Replay() *vaultsync.MembershipState {
	m, _ := vaultsync.NewMembershipState(cv.ID)
	for i := range cv.OpsSoFar {
		if err := m.Apply(&cv.OpsSoFar[i], cv.Check); err != nil {
			cv.T.Fatalf("an accepted op does not replay: %v", err)
		}
	}
	return m
}

// SignBatch signs ops in sequence on a scratch state, as one batch would carry them.
func (cv *ClientVault) SignBatch(u *User, ops ...vaultsync.MemberOp) []vaultsync.MemberOp {
	scratch := cv.Replay()
	signed := make([]vaultsync.MemberOp, 0, len(ops))
	for _, op := range ops {
		s := cv.SignOp(u, scratch, op)
		_ = scratch.Apply(&s, cv.Check)
		signed = append(signed, s)
	}
	return signed
}

// AppendOps sends a signed batch; the local state advances only if the server accepts.
func (cv *ClientVault) AppendOps(u *User, ops ...vaultsync.MemberOp) error {
	cv.T.Helper()
	signed := cv.SignBatch(u, ops...)
	if err := cv.H.Svc.AppendMembership(Ctx, cv.H.Principal(u.Access), cv.ID, signed); err != nil {
		return err
	}
	cv.Learn(signed...)
	return nil
}

// Learn records ops the server accepted by some other route (e.g. HTTP).
func (cv *ClientVault) Learn(ops ...vaultsync.MemberOp) {
	cv.OpsSoFar = append(cv.OpsSoFar, ops...)
	cv.M = cv.Replay()
}

// Record builds a signed envelope at the next version the local state expects.
func (cv *ClientVault) Record(u *User, record byte) vaultsync.Envelope {
	v, _ := cv.State.Version(vaultsync.ID{record})
	e := vaultsync.Envelope{Vault: cv.ID, Record: vaultsync.ID{record}, Version: v + 1, Epoch: cv.M.Epoch(), Suite: vaultsync.Suite1,
		WrappedDEK: bytes.Repeat([]byte{record}, 72), Body: bytes.Repeat([]byte{record}, 296), Author: u.Device, SigAlg: vaultsync.SigEd25519}
	msg, _ := e.SigningBytes()
	e.Sig = ed25519.Sign(u.DSK, msg)
	return e
}

// Commit builds a commit on the local state without applying it.
func (cv *ClientVault) Commit(u *User, records ...vaultsync.Envelope) vaultsync.Commit {
	cv.T.Helper()
	leaves := map[vaultsync.ID]vaultsync.Leaf{}
	for _, l := range cv.State.Leaves() {
		leaves[l.Record] = l
	}
	for i := range records {
		l, _ := vaultsync.LeafOf(&records[i])
		leaves[l.Record] = l
	}
	list := make([]vaultsync.Leaf, 0, len(leaves))
	for _, l := range leaves {
		list = append(list, l)
	}
	root, _ := vaultsync.MerkleRoot(list)
	cur := cv.State.Cursor()
	h := vaultsync.Head{Vault: cv.ID, Seq: cur.Seq + 1, PrevHash: cur.Hash, MerkleRoot: root, CurrentEpoch: cv.M.Epoch(),
		MembershipSeq: cv.M.Seq(), MembershipHead: cv.M.Hash(), Commit: RandomID(cv.T), Author: u.Device,
		ClientTimeUnixMilli: 1, SigAlg: vaultsync.SigEd25519}
	msg, _ := h.SigningBytes()
	h.Sig = ed25519.Sign(u.DSK, msg)
	return vaultsync.Commit{Head: h, Records: records}
}

// Push sends a commit through the use case on top of the local seq.
func (cv *ClientVault) Push(u *User, c vaultsync.Commit) (uint64, error) {
	return cv.H.Svc.Commit(Ctx, cv.H.Principal(u.Access), cv.ID, cv.State.Cursor().Seq, c)
}

// Adopt applies a commit the server accepted to the local state.
func (cv *ClientVault) Adopt(c vaultsync.Commit) {
	cv.T.Helper()
	next, err := cv.State.ApplyCommit(c, cv.M, cv.Resolve)
	if err != nil {
		cv.T.Fatalf("the server accepted a commit the client cannot verify: %v", err)
	}
	cv.State = next
}

// Accept pushes a commit that must succeed and adopts it.
func (cv *ClientVault) Accept(u *User, c vaultsync.Commit) uint64 {
	cv.T.Helper()
	seq, err := cv.Push(u, c)
	if err != nil {
		cv.T.Fatalf("commit by %s refused: %v", u.Name, err)
	}
	cv.Adopt(c)
	return seq
}

// NewTeamVault creates a vault owned by owner and adds the others with their roles, in the order
// given so the log is deterministic.
func (h *Harness) NewTeamVault(owner *User, others ...Member) *ClientVault {
	h.T.Helper()
	cv := &ClientVault{T: h.T, H: h, ID: RandomID(h.T), Users: map[vaultsync.ID]*User{owner.Account: owner}}
	cv.M, _ = vaultsync.NewMembershipState(cv.ID)
	cv.State, _ = vaultsync.NewVaultState(cv.ID)
	create := cv.SignOp(owner, cv.M, vaultsync.MemberOp{Kind: vaultsync.MemberCreate, RetentionDays: 30, Wraps: WrapFor(owner.Account)})
	if err := h.Svc.CreateVault(Ctx, h.Principal(owner.Access), create); err != nil {
		h.T.Fatalf("create vault: %v", err)
	}
	cv.Learn(create)
	for _, m := range others {
		cv.Users[m.User.Account] = m.User
		add := vaultsync.MemberOp{Kind: vaultsync.MemberAdd, Subject: m.User.Account, Role: m.Role, Verified: true,
			Epoch: cv.M.Epoch(), Wraps: WrapFor(m.User.Account)}
		if err := cv.AppendOps(owner, add); err != nil {
			h.T.Fatalf("add %s: %v", m.User.Name, err)
		}
	}
	return cv
}

// Member pairs a user with the role it joins a vault with.
type Member struct {
	User *User
	Role vaultsync.Role
}

// PersonalVault is the user's registered personal vault as the device tracks it.
func (h *Harness) PersonalVault(u *User) *ClientVault {
	h.T.Helper()
	cv := &ClientVault{T: h.T, H: h, ID: u.Vault, Users: map[vaultsync.ID]*User{u.Account: u}}
	cv.State, _ = vaultsync.NewVaultState(u.Vault)
	ops, err := h.Svc.Membership(Ctx, h.Principal(u.Access), u.Vault, 0)
	if err != nil {
		h.T.Fatal(err)
	}
	cv.Learn(ops...)
	return cv
}

// Tombstone builds a signed deletion at the next version the local state expects.
func (cv *ClientVault) Tombstone(u *User, record byte) vaultsync.Envelope {
	v, _ := cv.State.Version(vaultsync.ID{record})
	e := vaultsync.Envelope{Vault: cv.ID, Record: vaultsync.ID{record}, Version: v + 1, Epoch: cv.M.Epoch(), Suite: vaultsync.Suite1,
		Deleted: true, Author: u.Device, SigAlg: vaultsync.SigEd25519}
	msg, _ := e.SigningBytes()
	e.Sig = ed25519.Sign(u.DSK, msg)
	return e
}

// Purge builds a commit removing tombstones, its root over the state without them.
func (cv *ClientVault) Purge(u *User, records ...byte) vaultsync.Commit {
	cv.T.Helper()
	ids := make([]vaultsync.ID, len(records))
	for i, r := range records {
		ids[i] = vaultsync.ID{r}
	}
	var list []vaultsync.Leaf
	for _, l := range cv.State.Leaves() {
		if !slices.Contains(ids, l.Record) {
			list = append(list, l)
		}
	}
	root, _ := vaultsync.MerkleRoot(list)
	cur := cv.State.Cursor()
	h := vaultsync.Head{Vault: cv.ID, Seq: cur.Seq + 1, PrevHash: cur.Hash, MerkleRoot: root, CurrentEpoch: cv.M.Epoch(),
		MembershipSeq: cv.M.Seq(), MembershipHead: cv.M.Hash(), Commit: RandomID(cv.T), Author: u.Device,
		ClientTimeUnixMilli: 1, SigAlg: vaultsync.SigEd25519}
	msg, _ := h.SigningBytes()
	h.Sig = ed25519.Sign(u.DSK, msg)
	return vaultsync.Commit{Head: h, Purged: ids}
}
