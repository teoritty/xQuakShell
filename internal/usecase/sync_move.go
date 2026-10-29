package usecase

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

// MoveConnectionRequest names a connection and where it goes. To is a team vault's id as items carry
// it, or empty for the personal vault; Folder is a folder of that vault, or empty for its top level.
type MoveConnectionRequest struct {
	Connection string
	To         string
	Folder     string
}

// MoveConnection moves a connection into another synced vault - personal to team, team to personal
// or between teams - and returns the id it has there. Both ends must be writable by this account:
// the destination receives the connection, and the source loses it for every member.
//
// A move is a new item in the destination and a deletion in the source, and the new item gets a new
// id. Keeping the id made the two halves collide on every other device of the account: whichever
// vault synced first brought an item whose id the device still held in the other vault, and
// ApplyScope refuses that as a record reaching into another vault. For a move into the personal
// vault, which syncs first, that refusal repeated every round until the team caught up. The plugin
// secrets are copied under the new id for the same reason, and the forward rules get ids of their
// own, as a duplicate's do.
//
// The host is shared, the credential is not (spec §6.7): into a team, every user named with an
// inline key or password becomes a personal-source user this account binds to the credential it
// used, which stays in the personal vault. Out of a team, a user takes this account's binding back
// as an inline credential - unless the binding is suspended. A suspended binding means another
// member repointed the host, and copying the credential inline would hand it to that host without
// the confirmation the suspension exists to ask for, so such a user arrives with no credential.
func (e *SyncEngine) MoveConnection(ctx context.Context, req MoveConnectionRequest) (string, error) {
	data, err := e.d.Vault.GetData()
	if err != nil {
		return "", err
	}
	i := slices.IndexFunc(data.Connections, func(c domain.Connection) bool { return c.ID == req.Connection })
	if i < 0 {
		return "", domain.ErrConnectionNotFound
	}
	from := data.Connections[i].VaultID
	if from == req.To {
		return req.Connection, nil
	}
	for _, v := range []string{from, req.To} {
		if err := waitOutRateLimit(ctx, sleepCtx, func() error { return e.requireWritable(ctx, v) }); err != nil {
			return "", err
		}
	}
	id := newRandomID()
	err = e.d.Vault.UpdateData(ctx, func(d *domain.VaultData) error { return relocate(d, req, id) })
	if err != nil {
		return "", err
	}
	return id, nil
}

// moveRateLimitRetries and moveRateLimitMaxWait bound how long a move waits for the server's rate
// limit. A move is one click the person is watching: a few seconds of waiting reads as work, while
// "Too many attempts" in the middle of moving forty connections reads as breakage - and the limit is
// per address, so every member behind one office NAT spends from the same budget.
const (
	moveRateLimitRetries = 3
	moveRateLimitMaxWait = 10 * time.Second
)

// waitOutRateLimit runs fn and, while the server answers that it is rate limited, waits as long as
// it asked - capped - and runs fn again. Any other outcome, and the last refusal, is returned.
func waitOutRateLimit(ctx context.Context, sleep func(context.Context, time.Duration) error, fn func() error) error {
	err := fn()
	for range moveRateLimitRetries {
		var limited *synctrust.RateLimitedError
		if !errors.As(err, &limited) {
			return err
		}
		if serr := sleep(ctx, min(max(limited.RetryAfter, time.Second), moveRateLimitMaxWait)); serr != nil {
			return err
		}
		err = fn()
	}
	return err
}

func sleepCtx(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// requireWritable checks that this account may change vault, given as items carry it. The personal
// vault always is; a team must name a vault this device verified the membership of, with a role that
// writes. A viewer's change would stay on this device, and a move that half happened - gone from one
// vault for this device only - is worse than one refused.
func (e *SyncEngine) requireWritable(ctx context.Context, vault string) error {
	if vault == "" {
		return nil
	}
	id, err := vaultsync.ParseID(vault)
	if err != nil || e.scopeOf(id).Vault == "" {
		return ErrNotATeam
	}
	_, m, err := withMembers(e.startPull(ctx, id))
	if err != nil {
		return err
	}
	if !m.Role(e.d.Keys.Account()).CanWrite() {
		return vaultsync.ErrNotAuthorized
	}
	return nil
}

// relocate replaces the connection req names with a copy under id in the destination. Everything is
// checked before anything changes: UpdateData keeps what a failed mutation already wrote.
func relocate(d *domain.VaultData, req MoveConnectionRequest, id string) error {
	i := slices.IndexFunc(d.Connections, func(c domain.Connection) bool { return c.ID == req.Connection })
	if i < 0 {
		return domain.ErrConnectionNotFound
	}
	if req.Folder != "" {
		f := slices.IndexFunc(d.Folders, func(f domain.ConnectionFolder) bool { return f.ID == req.Folder })
		if f < 0 {
			return domain.ErrFolderNotFound
		}
		if d.Folders[f].VaultID != req.To {
			return domain.ErrVaultMismatch
		}
	}
	src := d.Connections[i]
	if err := checkHopsMovable(d, src, req.To); err != nil {
		return err
	}
	dup, err := domain.DuplicateConnection(src, id, src.Name, func() (string, error) { return newRandomID(), nil })
	if err != nil {
		return err
	}
	for _, from := range dup.SecretCopies {
		if _, ok := d.PluginSecrets[from]; !ok {
			return fmt.Errorf("move connection: secret %s is missing: %w", from, domain.ErrInvalidConnectionConfig)
		}
	}
	for to, from := range dup.SecretCopies {
		d.PluginSecrets[to] = d.PluginSecrets[from]
		delete(d.PluginSecrets, from)
	}
	moved := dup.Connection
	moved.VaultID, moved.FolderID, moved.Order = req.To, req.Folder, nextOrder(d.Connections, req.To, req.Folder)
	carryCredentials(d, &src, &moved)
	d.Connections = append(slices.Delete(d.Connections, i, i+1), moved)
	return nil
}

// checkHopsMovable refuses a move whose jump hops name a key or password from outside the
// destination. A user's credential becomes a binding on the way into a team, but a hop has no
// binding: its reference would travel in the shared record as it is, showing every member the id
// of this account's personal credential, and the destination would refuse to use it anyway.
func checkHopsMovable(d *domain.VaultData, src domain.Connection, to string) error {
	for _, hop := range src.JumpChain.Hops {
		if err := domain.CheckCredentialScope(d, to, hop.KeyAuth, hop.PassAuth); err != nil {
			return fmt.Errorf("move connection: jump host %s: %w", hop.Host, err)
		}
	}
	return nil
}

// carryCredentials gives moved the credentials src had, as the destination keeps them, and replaces
// this account's bindings to src with the ones moved needs.
func carryCredentials(d *domain.VaultData, src, moved *domain.Connection) {
	var keep []domain.CredentialBinding
	for j := range moved.Users {
		u := &moved.Users[j]
		old := domain.CredentialBinding{VaultID: src.VaultID, ConnectionID: src.ID, UserID: u.ID}
		switch {
		case moved.VaultID == "":
			restoreInline(d, src, u)
		case u.CredentialSource == domain.CredentialSourcePersonal:
			if b, ok := bindingOf(d, old.Key()); ok {
				b.VaultID, b.ConnectionID = moved.VaultID, moved.ID
				keep = append(keep, b)
			}
		default:
			if b, ok := bindingFor(moved, u); ok {
				keep = append(keep, b)
			}
		}
	}
	if d.Sync == nil && len(keep) == 0 {
		return
	}
	d.Sync = ensureSync(d.Sync)
	d.Sync.Bindings = slices.DeleteFunc(d.Sync.Bindings, func(b domain.CredentialBinding) bool {
		return b.VaultID == src.VaultID && b.ConnectionID == src.ID
	})
	d.Sync.Bindings = append(d.Sync.Bindings, keep...)
}

// restoreInline turns a personal-source user back into one naming its credential itself, from this
// account's binding. A missing or suspended binding leaves the user without a credential: the
// person picks one in the editor, as for a new connection.
func restoreInline(d *domain.VaultData, src *domain.Connection, u *domain.ConnectionUser) {
	if u.CredentialSource != domain.CredentialSourcePersonal {
		return
	}
	resolved, err := domain.ResolveCredentials(d, src, *u)
	if err != nil {
		resolved = *u
		resolved.KeyAuth, resolved.PassAuth = nil, nil
	}
	resolved.CredentialSource = ""
	*u = resolved
}

func bindingOf(d *domain.VaultData, key string) (domain.CredentialBinding, bool) {
	if d.Sync == nil {
		return domain.CredentialBinding{}, false
	}
	for _, b := range d.Sync.Bindings {
		if b.Key() == key {
			return b, true
		}
	}
	return domain.CredentialBinding{}, false
}

// nextOrder places a moved connection after the ones already at its new level, where the person
// who moved it will look for it, rather than wherever its old order happens to sort.
func nextOrder(conns []domain.Connection, vault, folder string) int {
	next := 0
	for _, c := range conns {
		if c.VaultID == vault && c.FolderID == folder && c.Order >= next {
			next = c.Order + 1
		}
	}
	return next
}
