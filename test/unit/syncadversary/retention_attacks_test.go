package syncadversary

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/syncwire"
)

// A server that slips a purge of a live record into a commit is refused: the head's Merkle root
// covers the state the author left, and a purge may only ever remove a tombstone.
func TestForgedPurgeOfALiveRecord(t *testing.T) {
	w, a, b := pair(t)
	var live string
	w.attack(changes(func(c *syncwire.Changes) {
		for _, e := range c.Commits {
			for _, r := range e.Records {
				if !r.Deleted {
					live = r.Record
				}
			}
		}
	}))
	a.addFolder("f2", "Dev")
	a.sync()
	b.sync()
	if live == "" {
		t.Fatal("the fixture saw no live record to aim at")
	}
	a.addFolder("f3", "QA")
	a.sync()
	w.attack(changes(func(c *syncwire.Changes) {
		for i := range c.Commits {
			c.Commits[i].Purged = append(c.Commits[i].Purged, live)
		}
	}))
	expectRefused(t, b, synctrust.ErrIntegrity)
}

// A server that claims history was pruned and then hands over a snapshot older than what the device
// verified is rolling it back; the resync refuses to step behind the verified head.
func TestPrunedHistoryWithAnOlderSnapshot(t *testing.T) {
	w, a, b := pair(t)
	var old []byte
	w.attack(func(r *http.Request, status int, body []byte) (int, []byte) {
		if strings.HasSuffix(r.URL.Path, "/snapshot") && status == http.StatusOK && old == nil {
			old = append([]byte(nil), body...)
		}
		return status, body
	})
	c := w.device("tablet")
	s, err := c.svc.Login(ctx, c.creds())
	if err != nil {
		t.Fatal(err)
	}
	c.use(s)
	if old == nil {
		t.Fatal("the fixture captured no snapshot")
	}
	a.addFolder("f2", "Dev")
	a.sync()
	b.sync()
	w.attack(func(r *http.Request, status int, body []byte) (int, []byte) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/changes"):
			raw, _ := json.Marshal(syncwire.ErrorBody{Error: syncwire.CodeHistoryPruned})
			return http.StatusGone, raw
		case strings.HasSuffix(r.URL.Path, "/snapshot"):
			return http.StatusOK, old
		}
		return status, body
	})
	expectRefused(t, b, synctrust.ErrIntegrity)
}

// A snapshot that drops a live record after a claimed pruning is refused: the records must reproduce
// the signed head's Merkle root, so a server cannot make a record disappear through a resync.
func TestPrunedHistoryWithASnapshotMissingARecord(t *testing.T) {
	w, a, b := pair(t)
	a.addFolder("f2", "Dev")
	a.sync()
	b.sync()
	a.addFolder("f3", "QA")
	a.sync()
	w.attack(func(r *http.Request, status int, body []byte) (int, []byte) {
		if strings.HasSuffix(r.URL.Path, "/changes") {
			raw, _ := json.Marshal(syncwire.ErrorBody{Error: syncwire.CodeHistoryPruned})
			return http.StatusGone, raw
		}
		return status, body
	})
	w.attack(on(suffix("/snapshot"), func(s *syncwire.Snapshot) {
		if len(s.Records) > 1 {
			s.Records = s.Records[1:]
		}
	}))
	expectRefused(t, b, synctrust.ErrIntegrity)
}
