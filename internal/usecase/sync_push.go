package usecase

import (
	"bytes"
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"maps"
	"slices"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// maxRebases bounds how often one sync pulls and retries after the vault moved on under it. A busy
// vault catches up on the next round; looping forever would hide a server that answers every commit
// with a conflict.
const maxRebases = 5

var (
	// ErrSyncBusy is a sync that lost the race maxRebases times in a row.
	ErrSyncBusy = errors.New("sync: the vault keeps moving; try again shortly")
	// ErrNoNamingKey is a push to a vault whose vault-meta this device has not seen.
	ErrNoNamingKey = errors.New("sync: the vault has no naming key yet")
)

// SyncReport is what one sync round did.
type SyncReport struct {
	Pull   PullReport
	Pushed int
}

// Sync runs one round for a vault: resend a commit a crash or a lost reply left in the outbox, pull,
// then push this device's changes, rebasing when another device got there first.
func (e *SyncEngine) Sync(ctx context.Context, vault vaultsync.ID) (SyncReport, error) {
	var report SyncReport
	for attempt := 0; attempt < maxRebases; attempt++ {
		p, err := e.startPull(ctx, vault)
		if err != nil {
			return report, err
		}
		if err := p.flushOutbox(ctx); err != nil {
			return report, err
		}
		err = p.pullAll(ctx)
		report.Pull = addPull(report.Pull, p.report)
		if err != nil {
			return report, err
		}
		pushed, err := p.push(ctx)
		var conflict *synctrust.ConflictError
		if errors.As(err, &conflict) {
			continue
		}
		report.Pushed += pushed
		return report, err
	}
	return report, ErrSyncBusy
}

func addPull(a, b PullReport) PullReport {
	return PullReport{Commits: a.Commits + b.Commits, Applied: a.Applied + b.Applied, Held: a.Held + b.Held,
		Quarantined: a.Quarantined + b.Quarantined}
}

// flushOutbox resends the commit a previous round recorded but never saw acknowledged. The server
// answers a commit id it already stored with that commit's seq, so a lost reply costs nothing; a
// conflict means it was never stored, and the vault still holds the edits for a fresh commit.
func (p *pullRun) flushOutbox(ctx context.Context) error {
	c, err := p.e.d.State.Outbox(ctx, p.vault)
	if err != nil || c == nil {
		return err
	}
	_, err = p.e.d.Server.Commit(ctx, *c)
	var conflict *synctrust.ConflictError
	if errors.As(err, &conflict) {
		return p.e.d.State.ClearOutbox(ctx, p.vault, c.Head.Commit)
	}
	if err != nil {
		return err
	}
	return p.adopt(ctx, *c)
}

// push commits what this device changed since the version it last verified.
func (p *pullRun) push(ctx context.Context) (int, error) {
	changes, err := p.localChanges(ctx)
	if err != nil || len(changes) == 0 {
		return 0, err
	}
	c, err := p.buildCommit(changes)
	if err != nil {
		return 0, err
	}
	// Recorded before it is sent: after a crash or a lost reply, the same commit with the same id is
	// resent rather than a second one built, which the server would store twice.
	if err := p.e.d.State.PutOutbox(ctx, c); err != nil {
		return 0, err
	}
	if _, err := p.e.d.Server.Commit(ctx, c); err != nil {
		var conflict *synctrust.ConflictError
		if errors.As(err, &conflict) {
			return 0, errors.Join(err, p.e.d.State.ClearOutbox(ctx, p.vault, c.Head.Commit))
		}
		return 0, err
	}
	return len(c.Records), p.adopt(ctx, c)
}

// adopt applies this device's own accepted commit through the same verification as anyone else's.
func (p *pullRun) adopt(ctx context.Context, c vaultsync.Commit) error {
	m, err := p.members.at(c.Head.MembershipSeq)
	if err != nil {
		return err
	}
	next, err := p.state.ApplyCommit(c, m, p.resolve)
	if err != nil {
		return integrity("own commit", err)
	}
	if err := p.e.d.State.AdvanceVault(ctx, c.Head, c.Records); err != nil {
		return err
	}
	p.advance(next, c.Records)
	return p.e.d.State.ClearOutbox(ctx, p.vault, c.Head.Commit)
}

// change is one record to write: a vault entry and the record id it is filed under.
type change struct {
	id    vaultsync.ID
	entry synctrust.Entry
}

// localChanges diffs the vault against the verified base. Records held or quarantined are left
// alone - pushing the local value over a held change would revert it everywhere, which only the
// user may decide - and so are records this build cannot read, which it must never overwrite or
// delete.
func (p *pullRun) localChanges(ctx context.Context) ([]change, error) {
	if p.namingKey == nil {
		return nil, ErrNoNamingKey
	}
	data, err := p.e.d.Vault.GetData()
	if err != nil {
		return nil, err
	}
	local, err := synctrust.Snapshot(p.namingKey, data)
	if err != nil {
		return nil, err
	}
	held, err := p.heldIDs(ctx)
	if err != nil {
		return nil, err
	}
	var out []change
	for id, le := range local {
		b, ok := p.baseBody(id)
		if held[id] || (ok && synctrust.SameData(b.Data, le.Data)) || (ok && !writable(b)) {
			continue
		}
		out = append(out, change{id: id, entry: le})
	}
	for id, env := range p.base {
		if _, present := local[id]; present || held[id] || env.Deleted || id == synctrust.VaultMetaID(p.vault) {
			continue
		}
		if b, ok := p.baseBody(id); ok && writable(b) {
			if key, err := synctrust.NaturalKey(b.Type, b.Data); err == nil {
				out = append(out, change{id: id, entry: synctrust.Entry{Type: b.Type, Key: key}})
			}
		}
	}
	slices.SortFunc(out, func(a, b change) int { return bytes.Compare(a.id[:], b.id[:]) })
	return out, nil
}

func writable(b synctrust.Body) bool {
	return vaultsync.RecordAccess(synctrust.KnownSchemas(), string(b.Type), b.Schema) == vaultsync.SchemaWritable
}

func (p *pullRun) heldIDs(ctx context.Context) (map[vaultsync.ID]bool, error) {
	items, err := p.e.d.State.Quarantined(ctx, p.vault)
	if err != nil {
		return nil, err
	}
	out := make(map[vaultsync.ID]bool, len(items))
	for _, it := range items {
		out[it.Envelope.Record] = true
	}
	return out, nil
}

// buildCommit seals and signs the changes and a head over them at the current membership position.
func (p *pullRun) buildCommit(changes []change) (vaultsync.Commit, error) {
	m, err := p.members.at(uint64(len(p.ops)))
	if err != nil {
		return vaultsync.Commit{}, err
	}
	records := make([]vaultsync.Envelope, 0, len(changes))
	for _, ch := range changes {
		env, err := p.sealChange(ch, m.Epoch())
		if err != nil {
			return vaultsync.Commit{}, err
		}
		records = append(records, env)
	}
	head, err := p.signHead(records, m)
	return vaultsync.Commit{Head: head, Records: records}, err
}

func (p *pullRun) sealChange(ch change, epoch uint32) (vaultsync.Envelope, error) {
	v, _ := p.state.Version(ch.id)
	env := vaultsync.Envelope{Vault: p.vault, Record: ch.id, Version: v + 1, Epoch: epoch, Suite: vaultsync.CurrentSuite,
		Deleted: ch.entry.Data == nil, Author: p.e.d.Keys.Device(), SigAlg: vaultsync.SigEd25519}
	if !env.Deleted {
		raw, err := synctrust.EncodeBody(synctrust.Body{Type: ch.entry.Type, Schema: 1, Data: ch.entry.Data})
		if err != nil {
			return env, err
		}
		vk, err := p.e.d.Keys.VaultKey(p.vault, epoch, p.ops)
		if err != nil {
			return env, err
		}
		if err := p.e.d.Cipher.Seal(vk, &env, raw); err != nil {
			return env, err
		}
	}
	msg, err := env.SigningBytes()
	if err != nil {
		return env, err
	}
	env.Sig, err = p.e.d.Keys.Sign(msg)
	return env, err
}

func (p *pullRun) signHead(records []vaultsync.Envelope, m *vaultsync.MembershipState) (vaultsync.Head, error) {
	leaves := map[vaultsync.ID]vaultsync.Leaf{}
	for _, l := range p.state.Leaves() {
		leaves[l.Record] = l
	}
	for i := range records {
		l, err := vaultsync.LeafOf(&records[i])
		if err != nil {
			return vaultsync.Head{}, err
		}
		leaves[l.Record] = l
	}
	root, err := vaultsync.MerkleRoot(slices.Collect(maps.Values(leaves)))
	if err != nil {
		return vaultsync.Head{}, err
	}
	commitID, err := vaultsync.NewID(rand.Reader)
	if err != nil {
		return vaultsync.Head{}, fmt.Errorf("commit id: %w", err)
	}
	cur := p.state.Cursor()
	h := vaultsync.Head{Vault: p.vault, Seq: cur.Seq + 1, PrevHash: cur.Hash, MerkleRoot: root, CurrentEpoch: m.Epoch(),
		MembershipSeq: m.Seq(), MembershipHead: m.Hash(), Commit: commitID, Author: p.e.d.Keys.Device(),
		ClientTimeUnixMilli: uint64(max(p.e.d.NowUnixMilli(), 0)), SigAlg: vaultsync.SigEd25519}
	msg, err := h.SigningBytes()
	if err != nil {
		return h, err
	}
	h.Sig, err = p.e.d.Keys.Sign(msg)
	return h, err
}
