package syncclient

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
)

var _ synctrust.SyncServer = (*Client)(nil)

// protocolErr marks a reply that decoded but does not convert into domain values.
func protocolErr(what string, err error) error {
	return fmt.Errorf("%w: %s: %w", synctrust.ErrRemoteProtocol, what, err)
}

func (c *Client) get(ctx context.Context, path string, query url.Values, reply any, limit int64) error {
	_, err := c.do(ctx, call{client: c.api, method: http.MethodGet, path: path, query: query, reply: reply, auth: true, limit: limit})
	return err
}

func (c *Client) post(ctx context.Context, path string, body, reply any) error {
	_, err := c.do(ctx, call{client: c.api, method: http.MethodPost, path: path, body: body, reply: reply, auth: true, limit: 0})
	return err
}

func since(n uint64) url.Values { return url.Values{"since": {strconv.FormatUint(n, 10)}} }

// Vaults lists the vaults the account belongs to.
func (c *Client) Vaults(ctx context.Context) ([]synctrust.RemoteVault, error) {
	var reply syncwire.VaultList
	if err := c.get(ctx, "/v1/vaults", nil, &reply, 0); err != nil {
		return nil, err
	}
	out := make([]synctrust.RemoteVault, 0, len(reply.Vaults))
	for _, v := range reply.Vaults {
		id, err := syncwire.ParseID(v.Vault)
		if err != nil {
			return nil, protocolErr("vault id", err)
		}
		out = append(out, synctrust.RemoteVault{Vault: id, Seq: v.Seq, MembershipSeq: v.MembershipSeq})
	}
	return out, nil
}

// CreateVault creates a vault with its genesis membership operation.
func (c *Client) CreateVault(ctx context.Context, genesis vaultsync.MemberOp) error {
	return c.post(ctx, "/v1/vaults", syncwire.VaultCreate{Op: syncwire.MemberOpFrom(&genesis)}, &struct{}{})
}

func vaultPath(vault vaultsync.ID, tail string) string {
	return "/v1/vaults/" + syncwire.HexID(vault) + "/" + tail
}

// Changes returns up to limit commits after seq since.
func (c *Client) Changes(ctx context.Context, vault vaultsync.ID, after uint64, limit int) ([]synctrust.RemoteCommit, bool, error) {
	q := since(after)
	q.Set("limit", strconv.Itoa(limit))
	var reply syncwire.Changes
	if err := c.get(ctx, vaultPath(vault, "changes"), q, &reply, bulkReplyLimit); err != nil {
		return nil, false, err
	}
	out := make([]synctrust.RemoteCommit, 0, len(reply.Commits))
	for _, e := range reply.Commits {
		commit, err := e.ToDomain()
		if err != nil {
			return nil, false, protocolErr("commit", err)
		}
		out = append(out, synctrust.RemoteCommit{Seq: e.Seq, Commit: commit})
	}
	return out, reply.More, nil
}

func toEnvelopes(records []syncwire.Envelope) ([]vaultsync.Envelope, error) {
	out := make([]vaultsync.Envelope, 0, len(records))
	for _, r := range records {
		e, err := r.ToDomain()
		if err != nil {
			return nil, protocolErr("envelope", err)
		}
		out = append(out, e)
	}
	return out, nil
}

// Snapshot returns the vault's current records and head.
func (c *Client) Snapshot(ctx context.Context, vault vaultsync.ID) (synctrust.RemoteSnapshot, error) {
	var reply syncwire.Snapshot
	if err := c.get(ctx, vaultPath(vault, "snapshot"), nil, &reply, bulkReplyLimit); err != nil {
		return synctrust.RemoteSnapshot{}, err
	}
	var out synctrust.RemoteSnapshot
	if reply.Head != nil {
		head, err := reply.Head.ToDomain()
		if err != nil {
			return synctrust.RemoteSnapshot{}, protocolErr("head", err)
		}
		out.Head = &head
	}
	envs, err := toEnvelopes(reply.Records)
	out.Records = envs
	return out, err
}

// Commit pushes a commit on top of c.Head.Seq-1.
func (c *Client) Commit(ctx context.Context, commit vaultsync.Commit) (uint64, error) {
	if commit.Head.Seq == 0 {
		return 0, fmt.Errorf("syncclient: commit at seq 0")
	}
	req := syncwire.CommitFrom(&commit)
	var reply syncwire.CommitReply
	_, err := c.do(ctx, call{client: c.api, method: http.MethodPost, path: vaultPath(commit.Head.Vault, "commit"),
		body: req, reply: &reply, auth: true})
	return reply.Seq, err
}

// DeviceLog returns an account's device-log entries after seq since.
func (c *Client) DeviceLog(ctx context.Context, account vaultsync.ID, after uint64) ([]vaultsync.DeviceEntry, error) {
	var reply syncwire.DeviceLog
	if err := c.get(ctx, "/v1/accounts/"+syncwire.HexID(account)+"/devicelog", since(after), &reply, 0); err != nil {
		return nil, err
	}
	out := make([]vaultsync.DeviceEntry, 0, len(reply.Entries))
	for _, w := range reply.Entries {
		e, err := w.ToDomain()
		if err != nil {
			return nil, protocolErr("device entry", err)
		}
		out = append(out, e)
	}
	return out, nil
}

// AppendDeviceLog appends entries to the signed-in account's device log.
func (c *Client) AppendDeviceLog(ctx context.Context, entries []vaultsync.DeviceEntry) error {
	req := syncwire.DeviceLogAppend{}
	for i := range entries {
		req.Entries = append(req.Entries, syncwire.DeviceEntryFrom(&entries[i]))
	}
	return c.post(ctx, "/v1/account/devicelog", req, &struct{}{})
}

// Membership returns a vault's membership operations after seq since.
func (c *Client) Membership(ctx context.Context, vault vaultsync.ID, after uint64) ([]vaultsync.MemberOp, error) {
	var reply syncwire.Membership
	if err := c.get(ctx, vaultPath(vault, "membership"), since(after), &reply, 0); err != nil {
		return nil, err
	}
	out := make([]vaultsync.MemberOp, 0, len(reply.Ops))
	for _, w := range reply.Ops {
		op, err := w.ToDomain()
		if err != nil {
			return nil, protocolErr("membership op", err)
		}
		out = append(out, op)
	}
	return out, nil
}

// AppendMembership appends operations to a vault's membership log.
func (c *Client) AppendMembership(ctx context.Context, vault vaultsync.ID, ops []vaultsync.MemberOp) error {
	req := syncwire.MembershipAppend{}
	for i := range ops {
		req.Ops = append(req.Ops, syncwire.MemberOpFrom(&ops[i]))
	}
	return c.post(ctx, vaultPath(vault, "membership"), req, &struct{}{})
}
