package syncclient

import (
	"context"
	"net/http"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
)

var _ synctrust.AccountServer = (*Client)(nil)

// public posts a request that carries no token: registration, login and device authentication run
// before the device has one.
func (c *Client) public(ctx context.Context, path string, body, reply any) error {
	_, err := c.do(ctx, call{client: c.api, method: http.MethodPost, path: path, body: body, reply: reply})
	return err
}

// RegisterInit starts an invite-only registration.
func (c *Client) RegisterInit(ctx context.Context, invite, username string, account vaultsync.ID, request []byte) ([]byte, error) {
	var reply syncwire.RegisterInitReply
	err := c.public(ctx, "/v1/register/init", syncwire.RegisterInit{Invite: invite, Username: username,
		Account: syncwire.HexID(account), OPAQUE: request}, &reply)
	return reply.OPAQUE, err
}

// RegisterFinish creates the account with its first device and personal vault.
func (c *Client) RegisterFinish(ctx context.Context, r synctrust.Registration) error {
	return c.public(ctx, "/v1/register/finish", syncwire.RegisterFinish{Invite: r.Invite, Username: r.Username,
		Account: syncwire.HexID(r.Account), Record: r.Record, IdentityKey: r.IdentityKey, EncryptionKey: r.EncryptionKey,
		Bundle: r.Bundle, Device: syncwire.DeviceEntryFrom(&r.Device), Vault: syncwire.MemberOpFrom(&r.Vault)}, &struct{}{})
}

// LoginInit starts a password login; for an unknown username the server answers with a fake
// account so the reply looks the same.
func (c *Client) LoginInit(ctx context.Context, username string, ke1 []byte) (string, vaultsync.ID, []byte, error) {
	var reply syncwire.LoginInitReply
	if err := c.public(ctx, "/v1/login/init", syncwire.LoginInit{Username: username, KE1: ke1}, &reply); err != nil {
		return "", vaultsync.ID{}, nil, err
	}
	account, err := syncwire.ParseID(reply.Account)
	if err != nil {
		return "", vaultsync.ID{}, nil, protocolErr("account id", err)
	}
	return reply.LoginID, account, reply.KE2, nil
}

// LoginFinish completes the login once the server verified KE3.
func (c *Client) LoginFinish(ctx context.Context, loginID string, ke3 []byte) (synctrust.LoginResult, error) {
	var reply syncwire.LoginFinishReply
	err := c.public(ctx, "/v1/login/finish", syncwire.LoginFinish{LoginID: loginID, KE3: ke3}, &reply)
	return synctrust.LoginResult{AccountToken: reply.AccountToken, Bundle: reply.Bundle, IdentityKey: reply.IdentityKey,
		EncryptionKey: reply.EncryptionKey}, err
}

// DeviceChallenge asks for a single-use nonce for this device to sign.
func (c *Client) DeviceChallenge(ctx context.Context, account, device vaultsync.ID) ([]byte, error) {
	var reply syncwire.DeviceChallengeReply
	err := c.public(ctx, "/v1/device/challenge", syncwire.DeviceChallenge{Account: syncwire.HexID(account),
		Device: syncwire.HexID(device)}, &reply)
	return reply.Nonce, err
}

// DeviceVerify exchanges the signed nonce for an access token.
func (c *Client) DeviceVerify(ctx context.Context, account, device vaultsync.ID, nonce, sig []byte) (string, error) {
	var reply syncwire.AccessReply
	err := c.public(ctx, "/v1/device/verify", syncwire.DeviceVerify{Account: syncwire.HexID(account),
		Device: syncwire.HexID(device), Nonce: nonce, Sig: sig}, &reply)
	return reply.AccessToken, err
}

// PasswordInit starts an OPAQUE re-registration for the signed-in account.
func (c *Client) PasswordInit(ctx context.Context, request []byte) ([]byte, error) {
	var reply syncwire.PasswordInitReply
	err := c.post(ctx, "/v1/account/password/init", syncwire.PasswordInit{OPAQUE: request}, &reply)
	return reply.OPAQUE, err
}

// PasswordFinish replaces the record and the bundle under the account key's signature.
func (c *Client) PasswordFinish(ctx context.Context, record, bundle, sig []byte) error {
	return c.post(ctx, "/v1/account/password/finish", syncwire.PasswordFinish{Record: record, Bundle: bundle, Sig: sig}, &struct{}{})
}
