package sshconfig

import (
	"sort"

	"xquakshell/internal/domain"
)

// noticeSet accumulates non-fatal parse findings, collapsing duplicates.
//
// Deduplication matters for usability rather than memory: a `Host *` block
// carrying an unreadable IdentityFile would otherwise produce one notice per
// host in the file, burying the handful of findings the user can act on.
type noticeSet struct {
	seen map[domain.SSHConfigNotice]bool
}

func newNoticeSet() *noticeSet {
	return &noticeSet{seen: map[domain.SSHConfigNotice]bool{}}
}

// add records a finding. An empty target is allowed: some findings are about
// the configuration as a whole rather than a named thing.
func (n *noticeSet) add(kind domain.SSHConfigNoticeKind, target string) {
	if n == nil {
		return
	}
	n.seen[domain.SSHConfigNotice{Kind: kind, Target: target}] = true
}

// list returns the collected notices in a stable order, so that repeated
// parses of the same file produce an identical result.
func (n *noticeSet) list() []domain.SSHConfigNotice {
	if n == nil || len(n.seen) == 0 {
		return nil
	}
	out := make([]domain.SSHConfigNotice, 0, len(n.seen))
	for notice := range n.seen {
		out = append(out, notice)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind != out[j].Kind {
			return out[i].Kind < out[j].Kind
		}
		return out[i].Target < out[j].Target
	})
	return out
}
