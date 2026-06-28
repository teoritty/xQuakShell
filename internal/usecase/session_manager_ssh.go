package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"ssh-client/internal/domain"
)

// connectSession performs the SSH handshake in a goroutine.
// Order: resolve auth (keys/password) → host key callback → optional jump chain
// transport → SSHClientFactory.Create → server-alive loop. Host key / passphrase UI is driven by
// HostKeyRequestFunc and PassphraseRequestFunc, not by this package.
// On host key errors it transitions to SessionHostKeyRequired and waits for RetrySession.
func (m *SessionManager) connectSession(entry *sessionEntry, conn *domain.Connection) {
	signers, password, err := m.resolveAuth(entry.ctx, conn)
	if err != nil {
		slog.Error("session auth failed", "sessionID", entry.info.SessionID, "err", err)
		m.updateState(entry, domain.SessionError, "Authentication failed")
		return
	}

	hostKeyCallback := m.hostKeyCB.Build(m.knownHosts)

	timeoutSec := 15
	if data, err := m.vaultRepo.GetData(); err == nil && data.Settings != nil && data.Settings.Transfer.ConnectionTimeoutSec > 0 {
		timeoutSec = data.Settings.Transfer.ConnectionTimeoutSec
	}

	sshCfg := domain.SSHClientConfig{
		Host:            conn.Host,
		Port:            conn.Port,
		User:            conn.EffectiveUsername(),
		Signers:         signers,
		Password:        password,
		HostKeyCallback: hostKeyCallback,
		TimeoutSeconds:  timeoutSec,
	}
	if !conn.JumpChain.IsEmpty() {
		hopResolver := func(hop domain.JumpHop) ([]domain.Signer, string, error) {
			return m.resolveHopAuthWithCtx(entry.ctx, hop)
		}
		transport, chainCleanup, chainErr := m.jumpTransport.BuildChain(
			entry.ctx,
			conn.JumpChain.Hops,
			conn.Host, conn.Port,
			timeoutSec,
			m.sshFactory,
			hostKeyCallback,
			hopResolver,
		)
		if chainErr != nil {
			if errors.Is(chainErr, domain.ErrUnknownHost) || errors.Is(chainErr, domain.ErrHostKeyMismatch) {
				m.handleHostKeyError(entry, conn, chainErr)
				return
			}
			slog.Error("session jump chain failed", "sessionID", entry.info.SessionID, "err", chainErr)
			m.updateState(entry, domain.SessionError, "Jump chain connection failed")
			return
		}
		sshCfg.Transport = transport

		go func() {
			<-entry.ctx.Done()
			chainCleanup()
		}()
	}

	client, err := m.sshFactory.Create(entry.ctx, sshCfg)
	if err != nil {
		if errors.Is(err, domain.ErrUnknownHost) || errors.Is(err, domain.ErrHostKeyMismatch) {
			m.handleHostKeyError(entry, conn, err)
			return
		}
		slog.Error("session SSH connect failed", "sessionID", entry.info.SessionID, "host", conn.Host, "err", err)
		m.updateState(entry, domain.SessionError, "Connection failed")
		return
	}

	m.mu.Lock()
	entry.sshClient = client
	m.mu.Unlock()

	go m.runServerAlive(entry)

	m.updateState(entry, domain.SessionReady, "")
}

func (m *SessionManager) handleHostKeyError(entry *sessionEntry, conn *domain.Connection, err error) {
	mismatch := errors.Is(err, domain.ErrHostKeyMismatch)

	hkInfo := domain.HostKeyInfo{
		Host:     fmt.Sprintf("%s:%d", conn.Host, conn.Port),
		Mismatch: mismatch,
	}

	var hkErr *domain.HostKeyVerificationError
	if errors.As(err, &hkErr) && hkErr != nil && hkErr.Info.Fingerprint != "" {
		hkInfo = hkErr.Info
		hkInfo.Mismatch = mismatch
	}

	m.mu.Lock()
	entry.hostKeyInfo = &hkInfo
	m.mu.Unlock()
	msg := "Host key verification required"
	if mismatch {
		msg = "Host key mismatch"
	}
	m.updateState(entry, domain.SessionHostKeyRequired, msg)
	if m.hostKeyRequest != nil {
		m.hostKeyRequest(entry.info.SessionID, hkInfo)
	}
}

// resolveHopAuthWithCtx resolves auth credentials for a single jump hop using the provided context.
func (m *SessionManager) resolveHopAuthWithCtx(ctx context.Context, hop domain.JumpHop) ([]domain.Signer, string, error) {
	switch hop.Auth {
	case domain.AuthMethodKey:
		if hop.KeyAuth == nil {
			return nil, "", nil
		}
		signers, err := m.loadSignersLegacy(ctx, hop.KeyAuth.IdentityIDs)
		return signers, "", err
	case domain.AuthMethodPassword:
		if hop.PassAuth == nil || hop.PassAuth.PasswordID == "" {
			return nil, "", fmt.Errorf("hop password auth but no password ID")
		}
		pw, err := m.passwordRepo.Get(ctx, hop.PassAuth.PasswordID)
		if err != nil {
			return nil, "", err
		}
		return nil, string(pw), nil
	default:
		return nil, "", nil
	}
}

// resolveAuth determines SSH auth (signers and/or password) from the connection's
// default user. Falls back to legacy IdentityIDs for v1 connections.
func (m *SessionManager) resolveAuth(ctx context.Context, conn *domain.Connection) ([]domain.Signer, string, error) {
	defaultUser := conn.DefaultUser()
	if defaultUser == nil {
		signers, err := m.loadSignersLegacy(ctx, conn.IdentityIDs)
		return signers, "", err
	}

	switch defaultUser.Auth {
	case domain.AuthMethodKey:
		if defaultUser.KeyAuth == nil {
			return nil, "", nil
		}
		signers, err := m.loadSignersLegacy(ctx, defaultUser.KeyAuth.IdentityIDs)
		return signers, "", err

	case domain.AuthMethodPassword:
		if defaultUser.PassAuth == nil || defaultUser.PassAuth.PasswordID == "" {
			return nil, "", fmt.Errorf("password auth configured but no password ID set")
		}
		passwordBytes, err := m.passwordRepo.Get(ctx, defaultUser.PassAuth.PasswordID)
		if err != nil {
			return nil, "", fmt.Errorf("load password: %w", err)
		}
		return nil, string(passwordBytes), nil

	default:
		return nil, "", fmt.Errorf("unknown auth method %q", defaultUser.Auth)
	}
}

// loadSignersLegacy reads private keys by their IDs and parses them into SSH signers.
func (m *SessionManager) loadSignersLegacy(ctx context.Context, identityIDs []string) ([]domain.Signer, error) {
	if len(identityIDs) == 0 {
		return nil, nil
	}

	signers := make([]domain.Signer, 0, len(identityIDs))
	for _, idRef := range identityIDs {
		pemData, err := m.identRepo.GetKeyBlob(ctx, idRef)
		if err != nil {
			return nil, fmt.Errorf("load key %s: %w", idRef, err)
		}

		passphrase, _ := m.passphraseCache.Get(idRef)
		signer, err := m.keySigner.ParsePrivateKeyWithPassphrase(pemData, passphrase)
		if err != nil {
			if err == domain.ErrPassphraseRequired && m.passphraseReq != nil {
				identMeta, metaErr := m.getIdentityMeta(ctx, idRef)
				comment := idRef
				if metaErr != nil {
					slog.Debug("getIdentityMeta failed", "id", idRef, "err", metaErr)
				} else if identMeta != nil {
					comment = identMeta.Comment
				}
				pp, ppErr := m.passphraseReq(idRef, comment)
				if ppErr != nil {
					return nil, fmt.Errorf("passphrase request for %s: %w", idRef, ppErr)
				}
				signer, err = m.keySigner.ParsePrivateKeyWithPassphrase(pemData, pp)
				if err != nil {
					return nil, fmt.Errorf("parse key %s with passphrase: %w", idRef, err)
				}
				m.passphraseCache.Set(idRef, pp)
			} else {
				return nil, fmt.Errorf("parse key %s: %w", idRef, err)
			}
		}

		signers = append(signers, signer)
	}
	return signers, nil
}

func (m *SessionManager) getIdentityMeta(ctx context.Context, id string) (*domain.SSHIdentity, error) {
	data, err := m.vaultRepo.GetData()
	if err != nil {
		return nil, err
	}
	ident, ok := data.Identities[id]
	if !ok {
		return nil, domain.ErrIdentityNotFound
	}
	return &ident, nil
}
