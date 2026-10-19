package usecase

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sync"

	"xquakshell/internal/domain/vaultsync"
)

const pendingLoginTTL = 2 * 60_000

type pendingLogin struct {
	account  vaultsync.ID
	username string
	ip       string
	known    bool
	mac      []byte
	expires  uint64
}

// pendingLogins holds the state between KE2 and KE3. It lives in memory only: a restart drops
// half-finished logins, which costs a user one retry and keeps OPAQUE state out of the database.
type pendingLogins struct {
	mu sync.Mutex
	m  map[string]pendingLogin
}

func newPendingLogins() *pendingLogins { return &pendingLogins{m: map[string]pendingLogin{}} }

func (p *pendingLogins) put(id string, l pendingLogin, now uint64) {
	p.mu.Lock()
	defer p.mu.Unlock()
	for k, v := range p.m {
		if v.expires <= now {
			delete(p.m, k)
		}
	}
	p.m[id] = l
}

// take removes and returns a pending login, so each one can be finished at most once.
func (p *pendingLogins) take(id string, now uint64) (pendingLogin, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	l, ok := p.m[id]
	delete(p.m, id)
	return l, ok && l.expires > now
}

// fakeAccountID is the id an unknown or frozen username answers with. It is keyed by a server
// secret, so it is stable per username - a real id that never changes must not stand out against
// a fake one that does - and unrelated to any real account.
func (s *Service) fakeAccountID(username string) vaultsync.ID {
	mac := hmac.New(sha256.New, s.cfg.FakeAccountSecret)
	mac.Write([]byte("fake-account\x00" + username))
	var id vaultsync.ID
	copy(id[:], mac.Sum(nil))
	return id
}

// LoginStart answers KE1. Unknown and frozen usernames get a fake record, so the reply has the same
// shape and the client fails the same way as on a wrong password.
//
// The attempt is charged to the limiter here, not when KE3 fails. KE2 is a guess oracle on its own:
// a client recovers the envelope from it and learns whether the password was right before it sends
// anything back, so a guesser that never sends KE3 would otherwise never be counted at all. Only a
// login that finishes refunds its charge.
func (s *Service) LoginStart(ctx context.Context, username, ip string, ke1 []byte) (string, vaultsync.ID, []byte, error) {
	if wait := s.limits.check(username, ip, s.now()); wait > 0 {
		return "", vaultsync.ID{}, nil, &RateLimitedError{RetryAfterMillis: wait}
	}
	s.limits.fail(username, ip, s.now())
	var acct Account
	var found bool
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		var err error
		acct, found, err = tx.AccountByUsername(username)
		return err
	})
	if err != nil {
		return "", vaultsync.ID{}, nil, err
	}
	pending := pendingLogin{username: username, ip: ip, expires: s.now() + pendingLoginTTL}
	var ke2 []byte
	if found && !acct.Frozen {
		pending.account, pending.known = acct.ID, true
		ke2, pending.mac, _, err = s.cfg.AKE.LoginStart(ke1, acct.OPAQUERecord, acct.ID)
	} else {
		pending.account = s.fakeAccountID(username)
		ke2, err = s.cfg.AKE.LoginStartUnknown(ke1, pending.account)
	}
	if err != nil {
		return "", vaultsync.ID{}, nil, malformed(err)
	}
	raw, err := s.randomBytes(16)
	if err != nil {
		return "", vaultsync.ID{}, nil, err
	}
	id := hex.EncodeToString(raw)
	s.logins.put(id, pending, s.now())
	return id, pending.account, ke2, nil
}

// LoginResult is returned only after KE3 verified.
type LoginResult struct {
	AccountToken  string
	Bundle        []byte
	IdentityKey   []byte
	EncryptionKey []byte
}

// LoginFinish verifies KE3. Every failure - unknown login id, expired, unknown user, wrong password,
// tampering - is the same ErrUnauthorized and keeps the charge LoginStart made; only a verified
// login refunds it.
func (s *Service) LoginFinish(ctx context.Context, loginID, ip string, ke3 []byte) (LoginResult, error) {
	pending, ok := s.logins.take(loginID, s.now())
	if !ok || pending.ip != ip {
		return LoginResult{}, ErrUnauthorized
	}
	if !pending.known || s.cfg.AKE.LoginFinish(ke3, pending.mac) != nil {
		return LoginResult{}, ErrUnauthorized
	}
	s.limits.succeed(pending.username, ip)
	var res LoginResult
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		acct, found, err := tx.AccountByID(pending.account)
		if err != nil || !found || acct.Frozen {
			return errors.Join(ErrUnauthorized, err)
		}
		token, _, err := s.issueToken(tx, acct.ID, vaultsync.ID{}, TokenAccount, accountTokenTTL)
		res = LoginResult{AccountToken: token, Bundle: acct.Bundle, IdentityKey: acct.IdentityKey, EncryptionKey: acct.EncryptionKey}
		return err
	})
	return res, err
}
