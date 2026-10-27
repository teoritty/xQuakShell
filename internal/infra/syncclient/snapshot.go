package syncclient

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/url"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
)

const (
	// snapshotRestarts bounds how often Snapshot starts over because the vault moved between pages.
	snapshotRestarts = 3
	// maxSnapshotRecords stops a server that pages without end. A vault on an honest server holds at
	// most 100 000 records by default; this allows an operator ten times that.
	maxSnapshotRecords = 1_000_000
)

// ErrSnapshotMoving is a vault that kept changing while its snapshot was read, page by page, every
// time. Nothing is wrong with what arrived - each page verified against its own head would - but no
// two pages agreed on one; the next round asks again, as it would after any unavailable server.
var ErrSnapshotMoving = fmt.Errorf("%w: the vault kept changing while its snapshot was read", synctrust.ErrRemoteInternal)

// errMovedBetweenPages is one attempt seeing the head change, before the attempts run out.
var errMovedBetweenPages = errors.New("the vault moved between snapshot pages")

// Snapshot returns the latest head and the latest version of every record, for a new device. The
// server serves it a page at a time, every page with the head it was read under; a snapshot is
// assembled only from pages that all name the same head, which is what makes them one state.
func (c *Client) Snapshot(ctx context.Context, vault vaultsync.ID) (synctrust.RemoteSnapshot, error) {
	for range snapshotRestarts {
		snap, err := c.snapshotOnce(ctx, vault)
		if !errors.Is(err, errMovedBetweenPages) {
			return snap, err
		}
	}
	return synctrust.RemoteSnapshot{}, ErrSnapshotMoving
}

func (c *Client) snapshotOnce(ctx context.Context, vault vaultsync.ID) (synctrust.RemoteSnapshot, error) {
	var out synctrust.RemoteSnapshot
	query := url.Values{}
	for page := 0; ; page++ {
		head, records, more, err := c.snapshotPage(ctx, vault, query)
		if err != nil {
			return synctrust.RemoteSnapshot{}, err
		}
		if page == 0 {
			out.Head = head
		} else if !sameHead(out.Head, head) {
			return synctrust.RemoteSnapshot{}, errMovedBetweenPages
		}
		if err := appendInOrder(&out, records); err != nil {
			return synctrust.RemoteSnapshot{}, err
		}
		if !more {
			return out, nil
		}
		// A page that claims more but carries nothing would be asked for again forever.
		if len(records) == 0 {
			return synctrust.RemoteSnapshot{}, protocolErr("snapshot", errors.New("an empty page claims more follow"))
		}
		query.Set("after", syncwire.HexID(records[len(records)-1].Record))
	}
}

func (c *Client) snapshotPage(ctx context.Context, vault vaultsync.ID, query url.Values) (*vaultsync.Head, []vaultsync.Envelope, bool, error) {
	var reply syncwire.Snapshot
	if err := c.get(ctx, vaultPath(vault, "snapshot"), query, &reply, bulkReplyLimit); err != nil {
		return nil, nil, false, err
	}
	var head *vaultsync.Head
	if reply.Head != nil {
		h, err := reply.Head.ToDomain()
		if err != nil {
			return nil, nil, false, protocolErr("head", err)
		}
		head = &h
	}
	records, err := toEnvelopes(reply.Records)
	return head, records, reply.More, err
}

// appendInOrder adds a page's records, which must continue the strictly increasing record order the
// pages are cut in: a record served twice, or out of order, is a server not paging as it was asked.
func appendInOrder(out *synctrust.RemoteSnapshot, records []vaultsync.Envelope) error {
	for i := range records {
		if n := len(out.Records); n > 0 && bytes.Compare(records[i].Record[:], out.Records[n-1].Record[:]) <= 0 {
			return protocolErr("snapshot", errors.New("records out of order across pages"))
		}
		out.Records = append(out.Records, records[i])
	}
	if len(out.Records) > maxSnapshotRecords {
		return protocolErr("snapshot", fmt.Errorf("more than %d records", maxSnapshotRecords))
	}
	return nil
}

func sameHead(a, b *vaultsync.Head) bool {
	if a == nil || b == nil {
		return a == b
	}
	ha, errA := a.Hash()
	hb, errB := b.Hash()
	return errA == nil && errB == nil && ha == hb
}
