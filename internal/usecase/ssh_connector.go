package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"ssh-client/internal/domain"
)

// WHY THIS TYPE IS SEPARATE FROM SessionManager (see ADR-009):
// Building an SSH connection (auth resolution, jump-chain, host-key
// verification) is a self-contained algorithm that has nothing to do with
// how a session is tracked or how plugin/embed sessions are bridged.
// Keeping it here means it can be unit-tested with a fake Connection and
// zero knowledge of sessionEntry, mutexes, or the plugin subsystem.
type SSHConnector struct {
	vaultRepo       domain.VaultRepository
	identRepo       domain.IdentityRepository
	passwordRepo    domain.PasswordRepository
	knownHosts      domain.KnownHostsRepository
	sshFactory      domain.SSHClientFactory
	passphraseCache domain.PassphraseCache
	hostKeyCB       domain.HostKeyCallbackBuilder
	jumpTransport   domain.JumpTransportBuilder
	keySigner       domain.PrivateKeySignerFactory
	passphraseReq   PassphraseRequestFunc
}

// SSHConnectorConfig holds dependencies for SSHConnector.
type SSHConnectorConfig struct {
	VaultRepo               domain.VaultRepository
	IdentRepo               domain.IdentityRepository
	PasswordRepo            domain.PasswordRepository
	KnownHosts              domain.KnownHostsRepository
	SSHFactory              domain.SSHClientFactory
	PassphraseCache         domain.PassphraseCache
	HostKeyCallbackBuilder  domain.HostKeyCallbackBuilder
	JumpTransportBuilder    domain.JumpTransportBuilder
	PrivateKeySignerFactory domain.PrivateKeySignerFactory
	PassphraseReq           PassphraseRequestFunc
}

// NewSSHConnector creates an SSH connector with the given dependencies.
func NewSSHConnector(cfg SSHConnectorConfig) *SSHConnector {
	return &SSHConnector{
		vaultRepo:       cfg.VaultRepo,
		identRepo:       cfg.IdentRepo,
		passwordRepo:    cfg.PasswordRepo,
		knownHosts:      cfg.KnownHosts,
		sshFactory:      cfg.SSHFactory,
		passphraseCache: cfg.PassphraseCache,
		hostKeyCB:       cfg.HostKeyCallbackBuilder,
		jumpTransport:   cfg.JumpTransportBuilder,
		keySigner:       cfg.PrivateKeySignerFactory,
		passphraseReq:   cfg.PassphraseReq,
	}
}

// ConnectResult holds the outcome of an SSH handshake attempt.
type ConnectResult struct {
	Client      domain.SSHClient
	HostKeyInfo *domain.HostKeyInfo // non-nil only when auth requires user decision
	JumpCleanup func()
	Err         error
}

// Connect performs the full handshake synchronously. The caller
// (SessionLifecycleService) is responsible for updating session state and
// starting the keepalive goroutine — this method has no side effects beyond
// the network connection itself.
func (c *SSHConnector) Connect(ctx context.Context, conn *domain.Connection) ConnectResult {
	signers, password, err := c.resolveAuth(ctx, conn)
	if err != nil {
		slog.Error("session auth failed", "host", conn.Host, "err", err)
		return ConnectResult{Err: fmt.Errorf("authentication failed: %w", err)}
	}

	hostKeyCallback := c.hostKeyCB.Build(c.knownHosts)

	timeoutSec := 15
	if data, err := c.vaultRepo.GetData(); err == nil && data.Settings != nil && data.Settings.Transfer.ConnectionTimeoutSec > 0 {
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

	var jumpCleanup func()
	if !conn.JumpChain.IsEmpty() {
		hopResolver := func(hop domain.JumpHop) ([]domain.Signer, string, error) {
			return c.resolveHopAuthWithCtx(ctx, hop)
		}
		transport, chainCleanup, chainErr := c.jumpTransport.BuildChain(
			ctx,
			conn.JumpChain.Hops,
			conn.Host, conn.Port,
			timeoutSec,
			c.sshFactory,
			hostKeyCallback,
			hopResolver,
		)
		if chainErr != nil {
			if hkInfo, ok := hostKeyInfoFromError(conn, chainErr); ok {
				return ConnectResult{HostKeyInfo: hkInfo, Err: chainErr}
			}
			slog.Error("session jump chain failed", "host", conn.Host, "err", chainErr)
			return ConnectResult{Err: fmt.Errorf("jump chain connection failed: %w", chainErr)}
		}
		sshCfg.Transport = transport
		jumpCleanup = chainCleanup
	}

	client, err := c.sshFactory.Create(ctx, sshCfg)
	if err != nil {
		if jumpCleanup != nil {
			jumpCleanup()
		}
		if hkInfo, ok := hostKeyInfoFromError(conn, err); ok {
			return ConnectResult{HostKeyInfo: hkInfo, Err: err}
		}
		slog.Error("session SSH connect failed", "host", conn.Host, "err", err)
		return ConnectResult{Err: fmt.Errorf("connection failed: %w", err)}
	}

	return ConnectResult{Client: client, JumpCleanup: jumpCleanup}
}

func hostKeyInfoFromError(conn *domain.Connection, err error) (*domain.HostKeyInfo, bool) {
	if !errors.Is(err, domain.ErrUnknownHost) && !errors.Is(err, domain.ErrHostKeyMismatch) {
		return nil, false
	}
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
	return &hkInfo, true
}

// resolveHopAuthWithCtx resolves auth credentials for a single jump hop using the provided context.
func (c *SSHConnector) resolveHopAuthWithCtx(ctx context.Context, hop domain.JumpHop) ([]domain.Signer, string, error) {
	switch hop.Auth {
	case domain.AuthMethodKey:
		if hop.KeyAuth == nil || len(hop.KeyAuth.IdentityIDs) == 0 {
			return nil, "", fmt.Errorf("hop key auth requires at least one identity")
		}
		signers, err := c.loadSigners(ctx, hop.KeyAuth.IdentityIDs)
		return signers, "", err
	case domain.AuthMethodPassword:
		if hop.PassAuth == nil || hop.PassAuth.PasswordID == "" {
			return nil, "", fmt.Errorf("hop password auth but no password ID")
		}
		pw, err := c.passwordRepo.Get(ctx, hop.PassAuth.PasswordID)
		if err != nil {
			return nil, "", err
		}
		return nil, string(pw), nil
	default:
		return nil, "", fmt.Errorf("hop unknown auth method %q", hop.Auth)
	}
}

// resolveAuth determines SSH auth (signers and/or password) from the connection's default user.
func (c *SSHConnector) resolveAuth(ctx context.Context, conn *domain.Connection) ([]domain.Signer, string, error) {
	defaultUser := conn.DefaultUser()
	if defaultUser == nil {
		return nil, "", fmt.Errorf("default user not configured")
	}

	switch defaultUser.Auth {
	case domain.AuthMethodKey:
		if defaultUser.KeyAuth == nil || len(defaultUser.KeyAuth.IdentityIDs) == 0 {
			return nil, "", fmt.Errorf("key auth requires at least one identity")
		}
		signers, err := c.loadSigners(ctx, defaultUser.KeyAuth.IdentityIDs)
		return signers, "", err

	case domain.AuthMethodPassword:
		if defaultUser.PassAuth == nil || defaultUser.PassAuth.PasswordID == "" {
			return nil, "", fmt.Errorf("password auth configured but no password ID set")
		}
		passwordBytes, err := c.passwordRepo.Get(ctx, defaultUser.PassAuth.PasswordID)
		if err != nil {
			return nil, "", fmt.Errorf("load password: %w", err)
		}
		return nil, string(passwordBytes), nil

	default:
		return nil, "", fmt.Errorf("unknown auth method %q", defaultUser.Auth)
	}
}

// loadSigners reads private keys by their IDs and parses them into SSH signers.
func (c *SSHConnector) loadSigners(ctx context.Context, identityIDs []string) ([]domain.Signer, error) {
	if len(identityIDs) == 0 {
		return nil, nil
	}

	signers := make([]domain.Signer, 0, len(identityIDs))
	for _, idRef := range identityIDs {
		pemData, err := c.identRepo.GetKeyBlob(ctx, idRef)
		if err != nil {
			return nil, fmt.Errorf("load key %s: %w", idRef, err)
		}

		passphrase, _ := c.passphraseCache.Get(idRef)
		signer, err := c.keySigner.ParsePrivateKeyWithPassphrase(pemData, passphrase)
		if err != nil {
			if err == domain.ErrPassphraseRequired && c.passphraseReq != nil {
				identMeta, metaErr := c.getIdentityMeta(ctx, idRef)
				comment := idRef
				if metaErr != nil {
					slog.Debug("getIdentityMeta failed", "id", idRef, "err", metaErr)
				} else if identMeta != nil {
					comment = identMeta.Comment
				}
				pp, ppErr := c.passphraseReq(idRef, comment)
				if ppErr != nil {
					return nil, fmt.Errorf("passphrase request for %s: %w", idRef, ppErr)
				}
				signer, err = c.keySigner.ParsePrivateKeyWithPassphrase(pemData, pp)
				if err != nil {
					return nil, fmt.Errorf("parse key %s with passphrase: %w", idRef, err)
				}
				c.passphraseCache.Set(idRef, pp)
			} else {
				return nil, fmt.Errorf("parse key %s: %w", idRef, err)
			}
		}

		signers = append(signers, signer)
	}
	return signers, nil
}

func (c *SSHConnector) getIdentityMeta(ctx context.Context, id string) (*domain.SSHIdentity, error) {
	data, err := c.vaultRepo.GetData()
	if err != nil {
		return nil, err
	}
	ident, ok := data.Identities[id]
	if !ok {
		return nil, domain.ErrIdentityNotFound
	}
	return &ident, nil
}
