package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
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
	vaultRepo         domain.VaultRepository
	identRepo         domain.IdentityRepository
	passwordRepo      domain.PasswordRepository
	knownHosts        domain.KnownHostsRepository
	sshFactory        domain.SSHClientFactory
	hostKeyCB         domain.HostKeyCallbackBuilder
	jumpTransport     domain.JumpTransportBuilder
	keys              *KeyManagerService
	passphraseReq     PassphraseRequestFunc
	authProvider      domain.PluginAuthProvider
	authMethodBuilder domain.PluginAuthMethodBuilder
	authAttempts      *PluginAuthAttemptRegistry
	authLookup        PluginAuthMethodLookup
	authStarter       PluginAuthStarter
	authGrant         PluginAuthGrantReader
}

// SSHConnectorConfig holds dependencies for SSHConnector.
type SSHConnectorConfig struct {
	VaultRepo              domain.VaultRepository
	IdentRepo              domain.IdentityRepository
	PasswordRepo           domain.PasswordRepository
	KnownHosts             domain.KnownHostsRepository
	SSHFactory             domain.SSHClientFactory
	PassphraseCache        domain.PassphraseCache
	HostKeyCallbackBuilder domain.HostKeyCallbackBuilder
	JumpTransportBuilder   domain.JumpTransportBuilder
	Keys                   *KeyManagerService
	PassphraseReq          PassphraseRequestFunc
	AuthProvider           domain.PluginAuthProvider
	AuthMethodBuilder      domain.PluginAuthMethodBuilder
	AuthAttempts           *PluginAuthAttemptRegistry
	AuthLookup             PluginAuthMethodLookup
	AuthStarter            PluginAuthStarter
	AuthGrantReader        PluginAuthGrantReader
}

// NewSSHConnector creates an SSH connector with the given dependencies.
func NewSSHConnector(cfg SSHConnectorConfig) *SSHConnector {
	return &SSHConnector{
		vaultRepo:         cfg.VaultRepo,
		identRepo:         cfg.IdentRepo,
		passwordRepo:      cfg.PasswordRepo,
		knownHosts:        cfg.KnownHosts,
		sshFactory:        cfg.SSHFactory,
		hostKeyCB:         cfg.HostKeyCallbackBuilder,
		jumpTransport:     cfg.JumpTransportBuilder,
		keys:              cfg.Keys,
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
		if hop.PassAuth == nil || hop.PassAuth.VaultRef == "" {
			return nil, "", nil, nil, fmt.Errorf("hop password auth but no password ID")
		}
		pw, err := c.passwordRepo.Get(ctx, hop.PassAuth.VaultRef)
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
		if defaultUser.PassAuth == nil || defaultUser.PassAuth.VaultRef == "" {
			return nil, "", nil, "", fmt.Errorf("password auth configured but no password ID set")
		}
		passwordBytes, err := c.passwordRepo.Get(ctx, defaultUser.PassAuth.VaultRef)
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

// loadSigners turns identity IDs into SSH signers, prompting for a passphrase where the key's
// own policy requires one.
//
// It goes through the key manager rather than parsing repository bytes itself. From schema 4 the
// stored bytes are always wrapped, and what unwraps them — a vault-held data key or the user's
// passphrase — is the key's policy to decide. Parsing here would have to re-implement that
// decision, and would get the per-key caching rules wrong the moment they diverged.
func (c *SSHConnector) loadSigners(ctx context.Context, identityIDs []string) ([]domain.Signer, error) {
	if len(identityIDs) == 0 {
		return nil, nil
	}

	signers := make([]domain.Signer, 0, len(identityIDs))
	for _, idRef := range identityIDs {
		signer, err := c.signerFor(ctx, idRef)
		if err != nil {
			return nil, err
		}
		signers = append(signers, signer)
	}
	return signers, nil
}

// maxPassphraseAttempts is how many passphrases one connection accepts for a key before giving up,
// the allowance OpenSSH's client gives. A mistyped passphrase is the common case and deserves
// another try without starting the connection over; an unbounded loop would instead keep a
// dialog on screen for as long as someone was willing to guess.
const maxPassphraseAttempts = 3

// signerFor produces one signer, asking the user for a passphrase only when the cached one is
// absent or no longer opens the key.
func (c *SSHConnector) signerFor(ctx context.Context, idRef string) (domain.Signer, error) {
	signer, err := c.keys.Signer(ctx, idRef)
	if err == nil {
		return signer, nil
	}
	needsPassphrase := errors.Is(err, domain.ErrPassphraseRequired) || errors.Is(err, domain.ErrKeyPassphraseWrong)
	if !needsPassphrase || c.passphraseReq == nil {
		return nil, fmt.Errorf("load key %s: %w", idRef, err)
	}

	question := PassphraseQuestion{IdentityID: idRef, Label: c.identityLabel(ctx, idRef)}
	for attempt := 1; ; attempt++ {
		pp, ppErr := c.passphraseReq(ctx, question)
		if ppErr != nil {
			return nil, fmt.Errorf("passphrase request for %s: %w", idRef, ppErr)
		}
		signer, err = c.keys.SignerWithPassphrase(ctx, idRef, pp)
		if !errors.Is(err, domain.ErrKeyPassphraseWrong) || attempt == maxPassphraseAttempts {
			break
		}
		slog.Warn("wrong key passphrase", "component", "session", "identity", idRef, "attempt", attempt)
		question.Retry = true
	}
	if err != nil {
		return nil, fmt.Errorf("parse key %s with passphrase: %w", idRef, err)
	}
	return signer, nil
}

// identityLabel is what the passphrase prompt calls the key. It falls back to the raw id rather
// than failing: being unable to name a key is no reason to refuse to ask for its passphrase.
func (c *SSHConnector) identityLabel(ctx context.Context, idRef string) string {
	identity, err := c.identRepo.Get(ctx, idRef)
	if err != nil {
		slog.Debug("identity lookup for passphrase prompt failed", "id", idRef, "err", err)
		return idRef
	}
	if identity.Comment == "" {
		return idRef
	}
	return identity.Comment
}
