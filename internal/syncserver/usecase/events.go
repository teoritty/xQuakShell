package usecase

import (
	"sync"

	"xquakshell/internal/domain/vaultsync"
)

const (
	maxSubscriptionsPerAccount = 16
	subscriptionBuffer         = 16
)

// Event tells a device something advanced, never what: it carries ids and positions only, and the
// device pulls and verifies the content itself.
type Event struct {
	Vault         vaultsync.ID
	Seq           uint64
	MembershipSeq uint64
	Account       vaultsync.ID
	DeviceLogSeq  uint64
}

type subscription struct {
	ch     chan Event
	closed bool
}

// hub fans events out to the subscribed devices of each account. Publishing never blocks: a
// subscriber whose buffer is full is dropped and its channel closed, so a stalled client can slow
// nobody's commit and simply reconnects and pulls.
type hub struct {
	mu   sync.Mutex
	subs map[vaultsync.ID]map[*subscription]struct{}
}

func newHub() *hub { return &hub{subs: map[vaultsync.ID]map[*subscription]struct{}{}} }

// Subscribe registers a device of account for events. The cancel function is safe to call twice
// and after the hub dropped the subscription.
func (s *Service) Subscribe(account vaultsync.ID) (<-chan Event, func(), error) {
	h := s.events
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.subs[account]) >= maxSubscriptionsPerAccount {
		return nil, nil, &RateLimitedError{RetryAfterMillis: 1_000}
	}
	sub := &subscription{ch: make(chan Event, subscriptionBuffer)}
	if h.subs[account] == nil {
		h.subs[account] = map[*subscription]struct{}{}
	}
	h.subs[account][sub] = struct{}{}
	return sub.ch, func() { h.drop(account, sub) }, nil
}

func (h *hub) drop(account vaultsync.ID, sub *subscription) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.dropLocked(account, sub)
}

func (h *hub) dropLocked(account vaultsync.ID, sub *subscription) {
	delete(h.subs[account], sub)
	if len(h.subs[account]) == 0 {
		delete(h.subs, account)
	}
	if !sub.closed {
		sub.closed = true
		close(sub.ch)
	}
}

func (h *hub) publish(accounts []vaultsync.ID, ev Event) {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, account := range accounts {
		for sub := range h.subs[account] {
			select {
			case sub.ch <- ev:
			default:
				h.dropLocked(account, sub)
			}
		}
	}
}
