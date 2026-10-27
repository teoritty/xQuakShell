package usecase

import (
	"context"
	"encoding/base64"
	"slices"

	"xquakshell/internal/domain/vaultsync"
)

// The limits one commit is cut to. The server refuses a commit of more than 1000 records and purges,
// or a body over its 20 MiB default, and nothing about a change set keeps it under either: an
// import, a bulk delete, and above all the re-encryption every rotation runs over each live record of
// a vault the server lets grow to 100 000. Each piece is an ordinary commit on top of the one before,
// so the protocol needs nothing new to carry them.
const (
	maxCommitRecords = 500
	// maxCommitWireBytes bounds the estimated encoded size of one commit, leaving a fifth of the
	// server's default body limit for the head and for what the estimate leaves out.
	maxCommitWireBytes = 16 << 20
	// envelopeWireOverhead covers an envelope's identifiers, integers and field names as JSON.
	envelopeWireOverhead = 512
)

// commitBatch is one commit being filled.
type commitBatch struct {
	records []vaultsync.Envelope
	purged  []vaultsync.ID
	wire    int
	// deletions marks a batch of tombstones, which never shares a commit with live versions.
	deletions bool
}

func (b *commitBatch) size() int { return len(b.records) + len(b.purged) }

// takes reports whether e may join the batch. A first record always does: a record too large for any
// commit then goes alone and is refused alone, rather than taking every change after it along.
func (b *commitBatch) takes(e *vaultsync.Envelope) bool {
	if b.size() == 0 {
		return true
	}
	return b.deletions == e.Deleted && b.size() < maxCommitRecords && b.wire+envelopeWireBytes(e) <= maxCommitWireBytes
}

func (b *commitBatch) add(e vaultsync.Envelope) {
	if b.size() == 0 {
		b.deletions = e.Deleted
	}
	b.records = append(b.records, e)
	b.wire += envelopeWireBytes(&e)
}

func envelopeWireBytes(e *vaultsync.Envelope) int {
	enc := base64.StdEncoding
	return enc.EncodedLen(len(e.WrappedDEK)) + enc.EncodedLen(len(e.Body)) + enc.EncodedLen(len(e.Sig)) + envelopeWireOverhead
}

// deletionsFirst orders the tombstones ahead of the live versions, each group as it came. The server
// charges a commit for the ciphertext it adds, so a person over a full quota gets back under it by
// deleting - and that works only if the deletions reach the server ahead of what it refuses.
func deletionsFirst(changes []change) []change {
	out := slices.Clone(changes)
	slices.SortStableFunc(out, func(a, b change) int {
		switch ad, bd := a.entry.Data == nil, b.entry.Data == nil; {
		case ad == bd:
			return 0
		case ad:
			return -1
		default:
			return 1
		}
	})
	return out
}

// commitAll seals the changes and sends them, and then the purges, as as many commits as the limits
// need, each built on the state the one before left. It returns how many records landed. The first
// refusal ends it: a conflict for the caller to rebase on, anything else to report, with the commits
// before it kept, since each was whole on its own.
func (p *pullRun) commitAll(ctx context.Context, changes []change, purged []vaultsync.ID) (int, error) {
	m, err := p.members.at(uint64(len(p.ops)))
	if err != nil {
		return 0, err
	}
	var b commitBatch
	sent := 0
	flush := func() error {
		if b.size() == 0 {
			return nil
		}
		head, err := p.signHead(b.records, b.purged, m)
		if err != nil {
			return err
		}
		if err := p.send(ctx, vaultsync.Commit{Head: head, Records: b.records, Purged: b.purged}); err != nil {
			return err
		}
		sent += len(b.records)
		b = commitBatch{}
		return nil
	}
	for _, ch := range deletionsFirst(changes) {
		env, err := p.sealChange(ch, m.Epoch())
		if err != nil {
			return sent, err
		}
		if !b.takes(&env) {
			if err := flush(); err != nil {
				return sent, err
			}
		}
		b.add(env)
	}
	for _, id := range purged {
		if b.size() >= maxCommitRecords {
			if err := flush(); err != nil {
				return sent, err
			}
		}
		b.purged = append(b.purged, id)
	}
	return sent, flush()
}
