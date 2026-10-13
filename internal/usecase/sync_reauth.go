package usecase

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// AuthenticatedServer keeps a device signed in: access tokens live fifteen minutes, so a request
// refused as unauthorized re-runs the device challenge with the device key and is retried once. A
// second refusal is returned as it is - a revoked device must stop, not loop.
type AuthenticatedServer struct {
	srv  synctrust.AccountServer
	keys synctrust.KeyRing

	mu        sync.Mutex
	connected bool
}

var (
	_ synctrust.SyncServer  = (*AuthenticatedServer)(nil)
	_ synctrust.EventSource = (*AuthenticatedServer)(nil)
)

// NewAuthenticatedServer wraps srv for the device keys holds.
func NewAuthenticatedServer(srv synctrust.AccountServer, keys synctrust.KeyRing) *AuthenticatedServer {
	return &AuthenticatedServer{srv: srv, keys: keys}
}

// Authenticate signs a fresh challenge and installs the access token it buys.
func (a *AuthenticatedServer) Authenticate(ctx context.Context) error {
	if err := a.connect(ctx, false); err != nil {
		return err
	}
	return authenticateDevice(ctx, a.srv, a.keys.Account(), a.keys.Device(), a.keys.Sign)
}

// connect runs hello, which verifies the server's TLS key against the pinned identity, once per
// session or again when forced. Building a session does not touch the network: it happens at
// unlock, and an unreachable server must not delay the vault opening.
func (a *AuthenticatedServer) connect(ctx context.Context, force bool) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.connected && !force {
		return nil
	}
	a.connected = false
	if _, err := a.srv.Hello(ctx); err != nil {
		return err
	}
	a.connected = true
	return nil
}

func authenticateDevice(ctx context.Context, srv synctrust.AccountServer, account, device vaultsync.ID, sign func([]byte) ([]byte, error)) error {
	nonce, err := srv.DeviceChallenge(ctx, account, device)
	if err != nil {
		return fmt.Errorf("device challenge: %w", err)
	}
	msg, err := vaultsync.DeviceAuthMessage(account, device, nonce)
	if err != nil {
		return err
	}
	sig, err := sign(msg)
	if err != nil {
		return err
	}
	token, err := srv.DeviceVerify(ctx, account, device, nonce, sig)
	if err != nil {
		return fmt.Errorf("device verify: %w", err)
	}
	srv.SetAccessToken(token)
	return nil
}

func withAuth[T any](ctx context.Context, a *AuthenticatedServer, f func() (T, error)) (T, error) {
	var zero T
	if err := a.connect(ctx, false); err != nil {
		return zero, err
	}
	v, err := f()
	// A renewed certificate fails the pin of every pooled connection; one fresh hello verifies the
	// new TLS key against the same identity, and a server that is not that identity fails it.
	if errors.Is(err, synctrust.ErrServerIdentity) {
		if cerr := a.connect(ctx, true); cerr != nil {
			return zero, errors.Join(err, cerr)
		}
		v, err = f()
	}
	if !errors.Is(err, synctrust.ErrRemoteUnauthorized) {
		return v, err
	}
	if aerr := a.Authenticate(ctx); aerr != nil {
		return zero, errors.Join(err, aerr)
	}
	return f()
}

func (a *AuthenticatedServer) Vaults(ctx context.Context) ([]synctrust.RemoteVault, error) {
	return withAuth(ctx, a, func() ([]synctrust.RemoteVault, error) { return a.srv.Vaults(ctx) })
}

func (a *AuthenticatedServer) CreateVault(ctx context.Context, op vaultsync.MemberOp) error {
	_, err := withAuth(ctx, a, func() (struct{}, error) { return struct{}{}, a.srv.CreateVault(ctx, op) })
	return err
}

type changesPage struct {
	commits []synctrust.RemoteCommit
	more    bool
}

func (a *AuthenticatedServer) Changes(ctx context.Context, vault vaultsync.ID, since uint64, limit int) ([]synctrust.RemoteCommit, bool, error) {
	p, err := withAuth(ctx, a, func() (changesPage, error) {
		c, more, err := a.srv.Changes(ctx, vault, since, limit)
		return changesPage{c, more}, err
	})
	return p.commits, p.more, err
}

func (a *AuthenticatedServer) Snapshot(ctx context.Context, vault vaultsync.ID) (synctrust.RemoteSnapshot, error) {
	return withAuth(ctx, a, func() (synctrust.RemoteSnapshot, error) { return a.srv.Snapshot(ctx, vault) })
}

func (a *AuthenticatedServer) Commit(ctx context.Context, c vaultsync.Commit) (uint64, error) {
	return withAuth(ctx, a, func() (uint64, error) { return a.srv.Commit(ctx, c) })
}

func (a *AuthenticatedServer) DeviceLog(ctx context.Context, account vaultsync.ID, since uint64) ([]vaultsync.DeviceEntry, error) {
	return withAuth(ctx, a, func() ([]vaultsync.DeviceEntry, error) { return a.srv.DeviceLog(ctx, account, since) })
}

func (a *AuthenticatedServer) AppendDeviceLog(ctx context.Context, entries []vaultsync.DeviceEntry) error {
	_, err := withAuth(ctx, a, func() (struct{}, error) { return struct{}{}, a.srv.AppendDeviceLog(ctx, entries) })
	return err
}

func (a *AuthenticatedServer) Membership(ctx context.Context, vault vaultsync.ID, since uint64) ([]vaultsync.MemberOp, error) {
	return withAuth(ctx, a, func() ([]vaultsync.MemberOp, error) { return a.srv.Membership(ctx, vault, since) })
}

func (a *AuthenticatedServer) AppendMembership(ctx context.Context, vault vaultsync.ID, ops []vaultsync.MemberOp) error {
	_, err := withAuth(ctx, a, func() (struct{}, error) { return struct{}{}, a.srv.AppendMembership(ctx, vault, ops) })
	return err
}

func (a *AuthenticatedServer) Events(ctx context.Context) (<-chan synctrust.RemoteEvent, error) {
	return withAuth(ctx, a, func() (<-chan synctrust.RemoteEvent, error) { return a.srv.Events(ctx) })
}
