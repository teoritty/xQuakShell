package vaultsync_test

import (
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

const day = uint64(24 * 60 * 60 * 1000)

func notInUse(uint32) bool { return false }

func TestEpochRetiresOneWindowAfterSuccessor(t *testing.T) {
	rots := []vaultsync.Rotation{{Epoch: 0, StartedUnixMilli: 0}, {Epoch: 1, StartedUnixMilli: 10 * day}}
	if vaultsync.EpochRetired(0, rots, notInUse, 30, 40*day-1) {
		t.Error("epoch 0 retired one millisecond early; trash written under it is still restorable")
	}
	if !vaultsync.EpochRetired(0, rots, notInUse, 30, 40*day) {
		t.Error("epoch 0 not retired exactly one window after epoch 1 began")
	}
}

func TestEpochNotRetiredWhileInUse(t *testing.T) {
	rots := []vaultsync.Rotation{{Epoch: 0}, {Epoch: 1, StartedUnixMilli: 1}}
	if vaultsync.EpochRetired(0, rots, func(e uint32) bool { return e == 0 }, 0, 100*day) {
		t.Error("an epoch a current record version still uses was retired; that record would become unreadable")
	}
}

func TestEpochNotRetiredWithoutSuccessor(t *testing.T) {
	if vaultsync.EpochRetired(1, []vaultsync.Rotation{{Epoch: 0}, {Epoch: 1}}, notInUse, 0, 100*day) {
		t.Error("the current epoch was retired")
	}
}

func TestEpochNotRetiredWhenClockIsBehind(t *testing.T) {
	rots := []vaultsync.Rotation{{Epoch: 0}, {Epoch: 1, StartedUnixMilli: 50 * day}}
	if vaultsync.EpochRetired(0, rots, notInUse, 0, 10*day) {
		t.Error("a clock behind the rotation retired the epoch; subtraction must not underflow into 'long ago'")
	}
}

func TestShredNowRetiresImmediately(t *testing.T) {
	rots := []vaultsync.Rotation{{Epoch: 0}, {Epoch: 1, StartedUnixMilli: 5 * day}}
	if !vaultsync.EpochRetired(0, rots, notInUse, 0, 5*day) {
		t.Error("with a zero window the previous epoch must retire as soon as the successor exists")
	}
}

func TestPurgeDue(t *testing.T) {
	if vaultsync.PurgeDue(10*day, 30, 40*day-1) {
		t.Error("purged before the window passed")
	}
	if !vaultsync.PurgeDue(10*day, 30, 40*day) {
		t.Error("not purged when the window passed")
	}
	if vaultsync.PurgeDue(10*day, 30, 1*day) {
		t.Error("a clock behind the deletion purged the record")
	}
}

func TestRotationDue(t *testing.T) {
	if vaultsync.RotationDue(0, false, 30, 100*day) {
		t.Error("rotation due for an unchanged vault")
	}
	if vaultsync.RotationDue(0, true, 0, 100*day) {
		t.Error("periodic rotation with a zero window would rotate on every sync")
	}
	if vaultsync.RotationDue(0, true, 30, 15*day-1) {
		t.Error("rotation due before half a window")
	}
	if !vaultsync.RotationDue(0, true, 30, 15*day) {
		t.Error("rotation not due at half a window")
	}
}
