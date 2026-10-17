package usecase

import (
	"context"
	"encoding/hex"
	"errors"
	"slices"
	"sync"

	"xquakshell/internal/domain/vaultsync"
)

const (
	challengeTTL   = 60_000
	challengeBytes = 32
)

type challenge struct {
	account, device vaultsync.ID
	expires         uint64
}

// challenges are single-use nonces for device proofs, held in memory for a minute.
type challenges struct {
	mu sync.Mutex
	m  map[string]challenge
}

func newChallenges() *challenges { return &challenges{m: map[string]challenge{}} }

// Principal is who a token speaks for.
type Principal struct {
	Account vaultsync.ID
	Device  vaultsync.ID
	Kind    TokenKind
}

// Authenticate resolves a bearer token of one of the given kinds. Frozen accounts are reported as
// such so the client can tell the user; revoked devices and expired tokens are just unauthorized.
func (s *Service) Authenticate(ctx context.Context, token string, kinds ...TokenKind) (Principal, error) {
	var p Principal
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		tok, ok, err := tx.TokenByHash(hashToken(token))
		if err != nil || !ok || tok.Expires <= s.now() || !slices.Contains(kinds, tok.Kind) {
			return errors.Join(ErrUnauthorized, err)
		}
		acct, ok, err := tx.AccountByID(tok.Account)
		if err != nil || !ok {
			return errors.Join(ErrUnauthorized, err)
		}
		if acct.Frozen {
			return ErrFrozen
		}
		if tok.Kind == TokenAccess {
			if d, ok, err := tx.DeviceByID(tok.Device); err != nil || !ok || d.Revoked {
				return errors.Join(ErrUnauthorized, err)
			}
		}
		p = Principal{Account: tok.Account, Device: tok.Device, Kind: tok.Kind}
		return nil
	})
	return p, err
}

// activeDevice loads a device of an unfrozen account that has not been revoked.
func activeDevice(tx Tx, account, device vaultsync.ID) (Device, error) {
	acct, ok, err := tx.AccountByID(account)
	if err != nil || !ok || acct.Frozen {
		return Device{}, errors.Join(ErrUnauthorized, err)
	}
	d, ok, err := tx.DeviceByID(device)
	if err != nil || !ok || d.Revoked || d.Account != account {
		return Device{}, errors.Join(ErrUnauthorized, err)
	}
	return d, nil
}

// DeviceChallenge issues a nonce for a certified, unrevoked device to sign.
func (s *Service) DeviceChallenge(ctx context.Context, account, device vaultsync.ID) ([]byte, error) {
	if err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		_, err := activeDevice(tx, account, device)
		return err
	}); err != nil {
		return nil, err
	}
	nonce, err := s.randomBytes(challengeBytes)
	if err != nil {
		return nil, err
	}
	s.challenges.mu.Lock()
	defer s.challenges.mu.Unlock()
	for k, c := range s.challenges.m {
		if c.expires <= s.now() {
			delete(s.challenges.m, k)
		}
	}
	s.challenges.m[hex.EncodeToString(nonce)] = challenge{account: account, device: device, expires: s.now() + challengeTTL}
	return nonce, nil
}

// takeChallenge consumes a nonce whether or not the proof that follows is valid, so a failed
// attempt cannot be retried against the same nonce.
func (s *Service) takeChallenge(account, device vaultsync.ID, nonce []byte) bool {
	s.challenges.mu.Lock()
	defer s.challenges.mu.Unlock()
	key := hex.EncodeToString(nonce)
	c, ok := s.challenges.m[key]
	delete(s.challenges.m, key)
	return ok && c.account == account && c.device == device && c.expires > s.now()
}

// DeviceVerify checks a device's signature over its nonce and issues an access token.
func (s *Service) DeviceVerify(ctx context.Context, account, device vaultsync.ID, nonce, sig []byte) (string, uint64, error) {
	if !s.takeChallenge(account, device, nonce) {
		return "", 0, ErrUnauthorized
	}
	msg, err := vaultsync.DeviceAuthMessage(account, device, nonce)
	if err != nil {
		return "", 0, err
	}
	var token string
	var expires uint64
	err = s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		d, err := activeDevice(tx, account, device)
		if err != nil {
			return err
		}
		if vaultsync.VerifySignature(vaultsync.SigEd25519, d.SigningKey, msg, sig) != nil {
			return ErrUnauthorized
		}
		token, expires, err = s.issueToken(tx, account, device, TokenAccess, accessTokenTTL)
		return err
	})
	return token, expires, err
}
