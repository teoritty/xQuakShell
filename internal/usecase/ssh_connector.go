package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"ssh-client/internal/domain"
	domainplugin "ssh-client/internal/domain/plugin"
)

// PluginAuthStarter ensures auth-provider plugins are running before handshake RPC.
type PluginAuthStarter interface {
	Activate(ctx context.Context, pluginID, reason string) error
}

// PluginAuthMethodLookup resolves auth method kinds from plugin manifests.
type PluginAuthMethodLookup interface {
	AuthMethodKind(pluginID, authMethodID string) (string, error)
	HasAuthProvider(pluginID string) (bool, error)
}

// SSHConnector performs SSH handshake with optional plugin-provided authentication.
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
	authProvider    domain.PluginAuthProvider
	authMethodBuilder domain.PluginAuthMethodBuilder
	authAttempts    *PluginAuthAttemptRegistry
	authLookup      PluginAuthMethodLookup
	authStarter     PluginAuthStarter
	authGrant       PluginAuthGrantReader
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
	AuthProvider            domain.PluginAuthProvider
	AuthMethodBuilder       domain.PluginAuthMethodBuilder
	AuthAttempts            *PluginAuthAttemptRegistry
	AuthLookup              PluginAuthMethodLookup
	AuthStarter             PluginAuthStarter
	AuthGrantReader         PluginAuthGrantReader
}

// NewSSHConnector creates an SSH connector with the given dependencies.
func NewSSHConnector(cfg SSHConnectorConfig) *SSHConnector {
	return &SSHConnector{
		vaultRepo:         cfg.VaultRepo,
		identRepo:         cfg.IdentRepo,
		passwordRepo:      cfg.PasswordRepo,
		knownHosts:        cfg.KnownHosts,
		sshFactory:        cfg.SSHFactory,
		passphraseCache:   cfg.PassphraseCache,
		hostKeyCB:         cfg.HostKeyCallbackBuilder,
		jumpTransport:     cfg.JumpTransportBuilder,
		keySigner:         cfg.PrivateKeySignerFactory,
		passphraseReq:     cfg.PassphraseReq,
		authProvider:      cfg.AuthProvider,
		authMethodBuilder: cfg.AuthMethodBuilder,
		authAttempts:      cfg.AuthAttempts,
		authLookup:        cfg.AuthLookup,
		authStarter:       cfg.AuthStarter,
		authGrant:         cfg.AuthGrantReader,
	}
}

// ConnectResult holds the outcome of an SSH handshake attempt.
type ConnectResult struct {
	Client      domain.SSHClient
	HostKeyInfo *domain.HostKeyInfo
	JumpCleanup func()
	Err         error
}

// Connect performs the full handshake synchronously.
func (c *SSHConnector) Connect(ctx context.Context, conn *domain.Connection) ConnectResult {
	signers, password, extraAuth, attemptID, err := c.resolveAuth(ctx, conn)
	if attemptID != "" && c.authAttempts != nil {
		defer c.authAttempts.End(attemptID)
	}
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
		Host:             conn.Host,
		Port:             conn.Port,
		User:             conn.EffectiveUsername(),
		Signers:          signers,
		Password:         password,
		ExtraAuthMethods: extraAuth,
		HostKeyCallback:  hostKeyCallback,
		TimeoutSeconds:   timeoutSec,
	}

	var jumpCleanup func()
	if !conn.JumpChain.IsEmpty() {
		hopResolver := func(hop domain.JumpHop) ([]domain.Signer, string, []domain.AuthMethod, func(), error) {
			return c.resolveHopAuthWithCtx(ctx, conn.ID, hop)
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

func (c *SSHConnector) resolveHopAuthWithCtx(ctx context.Context, connectionID string, hop domain.JumpHop) ([]domain.Signer, string, []domain.AuthMethod, func(), error) {
	switch hop.Auth {
	case domain.AuthMethodKey:
		if hop.KeyAuth == nil || len(hop.KeyAuth.IdentityIDs) == 0 {
			return nil, "", nil, nil, fmt.Errorf("hop key auth requires at least one identity")
		}
		signers, err := c.loadSigners(ctx, hop.KeyAuth.IdentityIDs)
		return signers, "", nil, nil, err
	case domain.AuthMethodPassword:
		if hop.PassAuth == nil || hop.PassAuth.PasswordID == "" {
			return nil, "", nil, nil, fmt.Errorf("hop password auth but no password ID")
		}
		pw, err := c.passwordRepo.Get(ctx, hop.PassAuth.PasswordID)
		if err != nil {
			return nil, "", nil, nil, err
		}
		return nil, string(pw), nil, nil, nil
	case domain.AuthMethodPlugin:
		extra, id, err := c.resolvePluginAuth(ctx, connectionID, hop.PluginAuth)
		if err != nil {
			return nil, "", nil, nil, err
		}
		var release func()
		if id != "" && c.authAttempts != nil {
			attemptID := id
			release = func() { c.authAttempts.End(attemptID) }
		}
		return nil, "", extra, release, nil
	default:
		return nil, "", nil, nil, fmt.Errorf("hop unknown auth method %q", hop.Auth)
	}
}

func (c *SSHConnector) resolveAuth(ctx context.Context, conn *domain.Connection) ([]domain.Signer, string, []domain.AuthMethod, string, error) {
	defaultUser := conn.DefaultUser()
	if defaultUser == nil {
		return nil, "", nil, "", fmt.Errorf("default user not configured")
	}

	switch defaultUser.Auth {
	case domain.AuthMethodKey:
		if defaultUser.KeyAuth == nil || len(defaultUser.KeyAuth.IdentityIDs) == 0 {
			return nil, "", nil, "", fmt.Errorf("key auth requires at least one identity")
		}
		signers, err := c.loadSigners(ctx, defaultUser.KeyAuth.IdentityIDs)
		return signers, "", nil, "", err

	case domain.AuthMethodPassword:
		if defaultUser.PassAuth == nil || defaultUser.PassAuth.PasswordID == "" {
			return nil, "", nil, "", fmt.Errorf("password auth configured but no password ID set")
		}
		passwordBytes, err := c.passwordRepo.Get(ctx, defaultUser.PassAuth.PasswordID)
		if err != nil {
			return nil, "", nil, "", fmt.Errorf("load password: %w", err)
		}
		return nil, string(passwordBytes), nil, "", nil

	case domain.AuthMethodPlugin:
		extra, attemptID, err := c.resolvePluginAuth(ctx, conn.ID, defaultUser.PluginAuth)
		return nil, "", extra, attemptID, err

	default:
		return nil, "", nil, "", fmt.Errorf("unknown auth method %q", defaultUser.Auth)
	}
}

func (c *SSHConnector) resolvePluginAuth(ctx context.Context, connectionID string, cfg *domain.PluginAuthConfig) ([]domain.AuthMethod, string, error) {
	if cfg == nil || cfg.PluginID == "" || cfg.AuthMethodID == "" {
		return nil, "", fmt.Errorf("plugin auth configured but no plugin auth config set")
	}
	if c.authProvider == nil || c.authMethodBuilder == nil || c.authAttempts == nil {
		return nil, "", fmt.Errorf("plugin auth is not configured in this build")
	}
	if c.authGrant == nil {
		return nil, "", fmt.Errorf("auth provider grant reader not configured")
	}
	if c.authLookup != nil {
		has, err := c.authLookup.HasAuthProvider(cfg.PluginID)
		if err != nil {
			return nil, "", err
		}
		if !has {
			return nil, "", fmt.Errorf("plugin %q is not an auth provider", cfg.PluginID)
		}
	}
	if !c.authGrant.IsAuthProviderGranted(cfg.PluginID) {
		return nil, "", fmt.Errorf("auth provider access not granted for plugin %q", cfg.PluginID)
	}
	if c.authStarter != nil {
		reason := "onAuthRequest:" + cfg.AuthMethodID
		if err := c.authStarter.Activate(ctx, cfg.PluginID, reason); err != nil {
			return nil, "", fmt.Errorf("start auth plugin: %w", err)
		}
	}
	attempt, err := c.authAttempts.Begin(cfg.PluginID, connectionID, cfg.AuthMethodID)
	if err != nil {
		return nil, "", fmt.Errorf("begin auth attempt: %w", err)
	}
	kind, err := c.authMethodKind(cfg.PluginID, cfg.AuthMethodID)
	if err != nil {
		return nil, attempt.ID, err
	}
	method := domain.PluginAuthMethod{
		PluginID:     attempt.PluginID,
		AuthMethodID: attempt.AuthMethodID,
		Kind:         kind,
		ConnectionID: connectionID,
		Fields:       cfg.Fields,
	}
	var authMethod domain.AuthMethod
	switch method.Kind {
	case domain.AuthProviderKindKeyboardInteractive:
		authMethod = c.authMethodBuilder.BuildKeyboardInteractive(ctx, c.authProvider, attempt.ID, method)
	case domain.AuthProviderKindPublicKey:
		authMethod, err = c.authMethodBuilder.BuildPublicKey(ctx, c.authProvider, attempt.ID, method)
		if err != nil {
			return nil, attempt.ID, err
		}
	default:
		return nil, attempt.ID, fmt.Errorf("unsupported plugin auth kind %q", method.Kind)
	}
	return []domain.AuthMethod{authMethod}, attempt.ID, nil
}

func (c *SSHConnector) authMethodKind(pluginID, authMethodID string) (string, error) {
	if c.authLookup == nil {
		return "", fmt.Errorf("auth method lookup unavailable")
	}
	kind, err := c.authLookup.AuthMethodKind(pluginID, authMethodID)
	if err != nil {
		return "", err
	}
	if kind != domain.AuthProviderKindKeyboardInteractive && kind != domain.AuthProviderKindPublicKey {
		return "", fmt.Errorf("%w: unknown auth kind %q", domainplugin.ErrInvalidManifest, kind)
	}
	return kind, nil
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
