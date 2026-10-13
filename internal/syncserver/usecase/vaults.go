package usecase

import (
	"context"
	"errors"
	"fmt"

	"xquakshell/internal/domain/vaultsync"
)

const maxOpsPerAppend = 16

// requireAccess refuses an account token where a device's access token is needed: everything a
// vault receives is signed by a device, and the server must know which one pushed it.
func requireAccess(p Principal) error {
	if p.Kind != TokenAccess || p.Device.IsZero() {
		return ErrUnauthorized
	}
	return nil
}

// requireMember reports a vault the caller is not in exactly like one that does not exist, so a
// vault id is never an existence oracle.
func requireMember(tx Tx, vault, account vaultsync.ID) (Vault, vaultsync.Role, error) {
	v, ok, err := tx.VaultByID(vault)
	if err != nil || !ok || v.Deleted {
		return Vault{}, vaultsync.RoleNone, errors.Join(ErrNotFound, err)
	}
	role, err := tx.MemberRole(vault, account)
	if err != nil || role == vaultsync.RoleNone {
		return Vault{}, vaultsync.RoleNone, errors.Join(ErrNotFound, err)
	}
	return v, role, nil
}

// signatureCheck verifies membership ops against the devices table. Historic ops by a device revoked
// since stay valid: whether a device's ops are accepted after its revocation is the membership log's
// own rule, applied by the replay.
func signatureCheck(tx Tx) vaultsync.SignatureCheck {
	return func(account, device vaultsync.ID, alg vaultsync.SigAlg, msg, sig []byte) error {
		d, ok, err := tx.DeviceByID(device)
		if err != nil || !ok || d.Account != account {
			return vaultsync.ErrBadSignature
		}
		return vaultsync.VerifySignature(alg, d.SigningKey, msg, sig)
	}
}

func deviceResolver(tx Tx) vaultsync.DeviceResolver {
	return func(device vaultsync.ID) (vaultsync.ID, []byte, error) {
		d, ok, err := tx.DeviceByID(device)
		if err != nil || !ok {
			return vaultsync.ID{}, nil, errors.Join(vaultsync.ErrDeviceLog, err)
		}
		return d.Account, d.SigningKey, nil
	}
}

func replayMembership(tx Tx, vault vaultsync.ID) (*vaultsync.MembershipState, error) {
	ops, err := tx.MemberOps(vault)
	if err != nil {
		return nil, err
	}
	m, err := vaultsync.NewMembershipState(vault)
	if err != nil {
		return nil, err
	}
	check := signatureCheck(tx)
	for i := range ops {
		if err := m.Apply(&ops[i], check); err != nil {
			return nil, fmt.Errorf("stored membership of %s: %w", vault, err)
		}
	}
	return m, nil
}

// refusal maps a vaultsync verdict on client input to the use-case error the transport reports: a
// role or revocation problem is forbidden, anything else is a malformed request.
func refusal(err error) error {
	if errors.Is(err, vaultsync.ErrNotAuthorized) || errors.Is(err, vaultsync.ErrRevokedDevice) {
		return fmt.Errorf("%w: %w", ErrForbidden, err)
	}
	return malformed(err)
}

func roles(m *vaultsync.MembershipState) map[vaultsync.ID]vaultsync.Role {
	out := map[vaultsync.ID]vaultsync.Role{}
	if m.Deleted() {
		return out
	}
	for _, id := range m.Members() {
		out[id] = m.Role(id)
	}
	return out
}

// CreateVault creates a team vault from its signed genesis operation.
func (s *Service) CreateVault(ctx context.Context, p Principal, create vaultsync.MemberOp) error {
	if err := requireAccess(p); err != nil {
		return err
	}
	if create.Kind != vaultsync.MemberCreate || create.Author != p.Account || create.AuthorDevice != p.Device {
		return ErrMalformed
	}
	return s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		if _, exists, err := tx.VaultByID(create.Vault); err != nil || exists {
			return errors.Join(ErrForbidden, err)
		}
		m, err := vaultsync.NewMembershipState(create.Vault)
		if err != nil {
			return err
		}
		if err := m.Apply(&create, signatureCheck(tx)); err != nil {
			return refusal(err)
		}
		if err := tx.InsertVault(Vault{ID: create.Vault, Owner: p.Account, MembershipSeq: 1}); err != nil {
			return err
		}
		if err := tx.AppendMemberOps(create.Vault, []vaultsync.MemberOp{create}); err != nil {
			return err
		}
		return tx.ReplaceMembers(create.Vault, roles(m))
	})
}

// AppendMembership verifies and appends a batch of membership operations by the caller's device. A
// batch ending with a removal not yet followed by a rotation is refused: the removed member must
// never be left holding the current key with no rotation queued.
func (s *Service) AppendMembership(ctx context.Context, p Principal, vault vaultsync.ID, ops []vaultsync.MemberOp) error {
	if err := requireAccess(p); err != nil {
		return err
	}
	if len(ops) == 0 || len(ops) > maxOpsPerAppend {
		return ErrMalformed
	}
	for _, op := range ops {
		if op.Author != p.Account || op.AuthorDevice != p.Device || op.Vault != vault {
			return ErrForbidden
		}
	}
	var notify []vaultsync.ID
	var seq uint64
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		if _, _, err := requireMember(tx, vault, p.Account); err != nil {
			return err
		}
		m, err := replayMembership(tx, vault)
		if err != nil {
			return err
		}
		notify = m.Members()
		check := signatureCheck(tx)
		for i := range ops {
			if err := m.Apply(&ops[i], check); err != nil {
				return refusal(err)
			}
		}
		if m.PendingRotation() {
			return malformed(fmt.Errorf("batch ends with a removal and no rotation"))
		}
		if err := tx.AppendMemberOps(vault, ops); err != nil {
			return err
		}
		if err := tx.SetVaultMembershipSeq(vault, m.Seq()); err != nil {
			return err
		}
		notify, seq = unionIDs(notify, m.Members()), m.Seq()
		return tx.ReplaceMembers(vault, roles(m))
	})
	// Members from before the batch are told too: a removed member's devices learn at once that they
	// must drop their copy of the vault.
	if err == nil {
		s.events.publish(notify, Event{Vault: vault, MembershipSeq: seq})
	}
	return err
}

func unionIDs(a, b []vaultsync.ID) []vaultsync.ID {
	seen := map[vaultsync.ID]bool{}
	var out []vaultsync.ID
	for _, id := range append(a, b...) {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}

// Membership returns a vault's membership log after since.
func (s *Service) Membership(ctx context.Context, p Principal, vault vaultsync.ID, since uint64) ([]vaultsync.MemberOp, error) {
	var out []vaultsync.MemberOp
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		if _, _, err := requireMember(tx, vault, p.Account); err != nil {
			return err
		}
		ops, err := tx.MemberOps(vault)
		for _, op := range ops {
			if op.Seq > since {
				out = append(out, op)
			}
		}
		return err
	})
	return out, err
}

// Vaults lists the vaults the caller belongs to.
func (s *Service) Vaults(ctx context.Context, p Principal) ([]Vault, error) {
	var out []Vault
	err := s.cfg.Store.WithTx(ctx, func(tx Tx) error {
		var err error
		out, err = tx.VaultsOf(p.Account)
		return err
	})
	return out, err
}
