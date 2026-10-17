package vaultsync

import "math"

func (s *MembershipState) applyKind(op *MemberOp) error {
	if op.Shred && op.Kind != MemberRotateEpoch {
		return rejected("shred on kind %d", op.Kind)
	}
	switch op.Kind {
	case MemberCreate:
		return s.applyCreate(op)
	case MemberAdd:
		return s.applyAdd(op)
	case MemberChangeRole:
		return s.applyChangeRole(op)
	case MemberRemove:
		return s.applyRemove(op)
	case MemberRotateEpoch:
		return s.applyRotate(op)
	case MemberRewrap:
		return s.applyRewrap(op)
	case MemberSetPolicy:
		return s.applySetPolicy(op)
	case MemberTransferOwnership:
		return s.applyTransfer(op)
	case MemberDeleteVault:
		return s.applyDelete(op)
	default:
		return rejected("unknown kind %d", op.Kind)
	}
}

func (s *MembershipState) requireManage(op *MemberOp) error {
	if !s.members[op.Author].CanManage() {
		return ErrNotAuthorized
	}
	return nil
}

func (s *MembershipState) requireOwner(op *MemberOp) error {
	if s.members[op.Author] != RoleOwner {
		return ErrNotAuthorized
	}
	return nil
}

// checkWraps requires exactly one wrap per wanted member and nobody else. A wrap that still carries
// its ciphertext must have the suite's size and match the digest the op signs; a pruned one is
// accepted, because the server strips retired epochs' wraps from logs every device replays.
func (s *MembershipState) checkWraps(wraps []KeyWrap, want []ID) error {
	if len(wraps) != len(want) {
		return rejected("%d wraps for %d members", len(wraps), len(want))
	}
	wanted := make(map[ID]bool, len(want))
	for _, id := range want {
		wanted[id] = true
	}
	for _, w := range wraps {
		if !wanted[w.Member] {
			return rejected("wrap for %s, who is not to receive one", w.Member)
		}
		if err := s.checkWrapContent(w); err != nil {
			return err
		}
		delete(wanted, w.Member)
	}
	return nil
}

func (s *MembershipState) checkWrapContent(w KeyWrap) error {
	if w.Pruned() {
		return nil
	}
	if len(w.Ciphertext) != s.params.VKWrapSize {
		return rejected("wrap size %d", len(w.Ciphertext))
	}
	d, err := wrapDigest(w.Ciphertext)
	if err != nil {
		return err
	}
	if d != w.Digest {
		return rejected("wrap for %s does not match its digest", w.Member)
	}
	return nil
}

// CheckNextEpoch refuses a rotation past the last epoch: a wrap to 0 would reuse the first epoch's
// coordinates in every AAD and HPKE info, so old ciphertext would open under the new key's labels.
func CheckNextEpoch(current uint32) error {
	if current == math.MaxUint32 {
		return rejected("epoch %d is the last", current)
	}
	return nil
}

func validRetention(days uint16) error {
	if days > MaxRetentionDays {
		return rejected("retention %d days above %d", days, MaxRetentionDays)
	}
	return nil
}

func (s *MembershipState) applyCreate(op *MemberOp) error {
	if op.Epoch != 0 {
		return rejected("create at epoch %d", op.Epoch)
	}
	if err := validRetention(op.RetentionDays); err != nil {
		return err
	}
	if err := s.checkWraps(op.Wraps, []ID{op.Author}); err != nil {
		return err
	}
	s.members[op.Author] = RoleOwner
	s.owner, s.creator = op.Author, op.Author
	s.retentionDays = op.RetentionDays
	s.requireVerified = op.RequireVerified
	s.rotations = append(s.rotations, Rotation{Epoch: 0, StartedUnixMilli: op.TimeUnixMilli})
	return nil
}

func assignableRole(r Role) bool { return r >= RoleViewer && r <= RoleAdmin }

func (s *MembershipState) applyAdd(op *MemberOp) error {
	if err := s.requireManage(op); err != nil {
		return err
	}
	if op.Subject.IsZero() || s.members[op.Subject] != RoleNone || !assignableRole(op.Role) {
		return rejected("cannot add %s as role %d", op.Subject, op.Role)
	}
	if s.requireVerified && !op.Verified {
		return ErrUnverifiedMember
	}
	if op.Fingerprint == ([32]byte{}) {
		return rejected("add of %s records no key fingerprint", op.Subject)
	}
	if op.Epoch != s.epoch {
		return rejected("add at epoch %d, current %d", op.Epoch, s.epoch)
	}
	if err := s.checkWraps(op.Wraps, []ID{op.Subject}); err != nil {
		return err
	}
	s.members[op.Subject] = op.Role
	s.fingerprints[op.Subject] = op.Fingerprint
	return nil
}

func (s *MembershipState) applyChangeRole(op *MemberOp) error {
	if err := s.requireManage(op); err != nil {
		return err
	}
	current := s.members[op.Subject]
	if current == RoleNone || current == RoleOwner || !assignableRole(op.Role) {
		return rejected("cannot change %s from %d to %d", op.Subject, current, op.Role)
	}
	s.members[op.Subject] = op.Role
	return nil
}

func (s *MembershipState) applyRemove(op *MemberOp) error {
	current := s.members[op.Subject]
	if current == RoleNone || current == RoleOwner {
		return rejected("cannot remove %s with role %d", op.Subject, current)
	}
	if op.Author != op.Subject {
		if err := s.requireManage(op); err != nil {
			return err
		}
	}
	delete(s.members, op.Subject)
	s.pendingRotation = true
	return nil
}

func (s *MembershipState) applyRotate(op *MemberOp) error {
	if err := s.requireManage(op); err != nil {
		return err
	}
	if err := CheckNextEpoch(s.epoch); err != nil {
		return err
	}
	if op.Epoch != s.epoch+1 {
		return rejected("rotation to epoch %d from %d", op.Epoch, s.epoch)
	}
	// Retirement counts from rotation times, so a rotation dated before the previous one would
	// retire that epoch's keys early, before its trash and history had aged out.
	if last := s.rotations[len(s.rotations)-1]; op.TimeUnixMilli < last.StartedUnixMilli {
		return rejected("rotation at %d before the previous one at %d", op.TimeUnixMilli, last.StartedUnixMilli)
	}
	if err := s.checkWraps(op.Wraps, s.Members()); err != nil {
		return err
	}
	for _, d := range op.RevokedDevices {
		s.revoked[d] = struct{}{}
	}
	s.epoch = op.Epoch
	s.rotations = append(s.rotations, Rotation{Epoch: op.Epoch, StartedUnixMilli: op.TimeUnixMilli, Shred: op.Shred})
	s.pendingRotation = false
	return nil
}

func (s *MembershipState) applyRewrap(op *MemberOp) error {
	if err := s.requireManage(op); err != nil {
		return err
	}
	if s.members[op.Subject] == RoleNone || op.Epoch != s.epoch {
		return rejected("rewrap for %s at epoch %d", op.Subject, op.Epoch)
	}
	return s.checkWraps(op.Wraps, []ID{op.Subject})
}

func (s *MembershipState) applySetPolicy(op *MemberOp) error {
	if err := s.requireOwner(op); err != nil {
		return err
	}
	if err := validRetention(op.RetentionDays); err != nil {
		return err
	}
	s.retentionDays = op.RetentionDays
	s.requireVerified = op.RequireVerified
	return nil
}

func (s *MembershipState) applyTransfer(op *MemberOp) error {
	if err := s.requireOwner(op); err != nil {
		return err
	}
	current := s.members[op.Subject]
	if current == RoleNone || current == RoleOwner {
		return rejected("cannot transfer ownership to %s", op.Subject)
	}
	s.members[op.Author] = RoleAdmin
	s.members[op.Subject] = RoleOwner
	s.owner = op.Subject
	return nil
}

func (s *MembershipState) applyDelete(op *MemberOp) error {
	if err := s.requireOwner(op); err != nil {
		return err
	}
	s.deleted = true
	return nil
}
