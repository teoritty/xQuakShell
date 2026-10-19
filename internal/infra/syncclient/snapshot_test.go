package syncclient_test

import (
	"bytes"
	"errors"
	"net/http"
	"sync/atomic"
	"testing"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/syncclient"
)

func snapEnvelope(record byte) syncwire.Envelope {
	e := vaultsync.Envelope{Vault: vaultsync.ID{1}, Record: vaultsync.ID{record}, Version: 1, Epoch: 0, Suite: vaultsync.Suite1,
		WrappedDEK: bytes.Repeat([]byte{5}, 72), Body: bytes.Repeat([]byte{6}, 296), Author: vaultsync.ID{7},
		SigAlg: vaultsync.SigEd25519, Sig: bytes.Repeat([]byte{8}, 64)}
	return syncwire.EnvelopeFrom(&e)
}

func snapHead(seq uint64) *syncwire.Head {
	h := vaultsync.Head{Vault: vaultsync.ID{1}, Seq: seq, Commit: vaultsync.ID{3}, Author: vaultsync.ID{7},
		SigAlg: vaultsync.SigEd25519, Sig: bytes.Repeat([]byte{8}, 64)}
	w := syncwire.HeadFrom(&h)
	return &w
}

// pagedSnapshot serves pages keyed by the after parameter, "" for the first.
func pagedSnapshot(f *fakeServer, pages map[string]func() syncwire.Snapshot) {
	f.mux.HandleFunc("GET /v1/vaults/{id}/snapshot", func(w http.ResponseWriter, r *http.Request) {
		page, ok := pages[r.URL.Query().Get("after")]
		if !ok {
			writeJSON(w, http.StatusBadRequest, syncwire.ErrorBody{Error: syncwire.CodeMalformed})
			return
		}
		writeJSON(w, http.StatusOK, page())
	})
}

func fixed(s syncwire.Snapshot) func() syncwire.Snapshot { return func() syncwire.Snapshot { return s } }

// A snapshot arrives a page at a time; the client asks for each next page after the last record of
// the one before and assembles one state.
func TestSnapshotAssemblesItsPages(t *testing.T) {
	f := newFake(t)
	pagedSnapshot(f, map[string]func() syncwire.Snapshot{
		"": fixed(syncwire.Snapshot{Head: snapHead(4), Records: []syncwire.Envelope{snapEnvelope(1), snapEnvelope(2)}, More: true}),
		syncwire.HexID(vaultsync.ID{2}): fixed(syncwire.Snapshot{Head: snapHead(4), Records: []syncwire.Envelope{snapEnvelope(3)}}),
	})
	snap, err := helloed(t, f).Snapshot(ctx, vaultsync.ID{1})
	if err != nil || snap.Head == nil || snap.Head.Seq != 4 || len(snap.Records) != 3 || snap.Records[2].Record != (vaultsync.ID{3}) {
		t.Fatalf("snapshot = %+v, %v; want head 4 and records 1, 2, 3", snap, err)
	}
}

// Pages read under different heads are not one state. The client starts over, and a vault that keeps
// moving is an unavailable server for this round - never a mix of two states.
func TestSnapshotStartsOverWhenTheVaultMoves(t *testing.T) {
	f := newFake(t)
	var firsts atomic.Int32
	pagedSnapshot(f, map[string]func() syncwire.Snapshot{
		"": func() syncwire.Snapshot {
			firsts.Add(1)
			return syncwire.Snapshot{Head: snapHead(4), Records: []syncwire.Envelope{snapEnvelope(1)}, More: true}
		},
		syncwire.HexID(vaultsync.ID{1}): fixed(syncwire.Snapshot{Head: snapHead(5), Records: []syncwire.Envelope{snapEnvelope(2)}}),
	})
	_, err := helloed(t, f).Snapshot(ctx, vaultsync.ID{1})
	if !errors.Is(err, syncclient.ErrSnapshotMoving) || !errors.Is(err, synctrust.ErrRemoteInternal) {
		t.Fatalf("snapshot of a vault that moves between every two pages: %v, want ErrSnapshotMoving", err)
	}
	if n := firsts.Load(); n != 3 {
		t.Fatalf("started %d times; want three attempts", n)
	}
}

// A server that serves a record twice, out of order, or an empty page claiming more is not paging
// as asked, and is refused rather than followed.
func TestSnapshotRefusesBrokenPaging(t *testing.T) {
	cases := map[string]map[string]func() syncwire.Snapshot{
		"out of order": {
			"": fixed(syncwire.Snapshot{Head: snapHead(4), Records: []syncwire.Envelope{snapEnvelope(2), snapEnvelope(1)}}),
		},
		"repeated across pages": {
			"": fixed(syncwire.Snapshot{Head: snapHead(4), Records: []syncwire.Envelope{snapEnvelope(1), snapEnvelope(2)}, More: true}),
			syncwire.HexID(vaultsync.ID{2}): fixed(syncwire.Snapshot{Head: snapHead(4), Records: []syncwire.Envelope{snapEnvelope(2)}}),
		},
		"empty page claiming more": {
			"": fixed(syncwire.Snapshot{Head: snapHead(4), Records: []syncwire.Envelope{}, More: true}),
		},
	}
	for name, pages := range cases {
		t.Run(name, func(t *testing.T) {
			f := newFake(t)
			pagedSnapshot(f, pages)
			if _, err := helloed(t, f).Snapshot(ctx, vaultsync.ID{1}); !errors.Is(err, synctrust.ErrRemoteProtocol) {
				t.Fatalf("err = %v, want ErrRemoteProtocol", err)
			}
		})
	}
}
