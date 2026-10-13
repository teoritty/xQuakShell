package vaultsync

const (
	// DefaultRetentionDays is how long deleted records and old versions stay restorable.
	DefaultRetentionDays uint16 = 30
	// MaxRetentionDays caps the window: a year of history is the most a vault may keep.
	MaxRetentionDays uint16 = 365
)

// Rotation records when a vault epoch began, according to the signed clock of the device that
// rotated. Retirement of the previous epoch is counted from it.
type Rotation struct {
	Epoch            uint32
	StartedUnixMilli uint64
}

const dayMillis uint64 = 24 * 60 * 60 * 1000

func windowMillis(days uint16) uint64 { return uint64(days) * dayMillis }

// elapsed reports whether at least d has passed since start. A clock earlier than start counts as
// no time passed: the conservative answer keeps keys and records rather than destroying them.
func elapsed(start, now, d uint64) bool {
	return now >= start && now-start >= d
}

// EpochRetired reports whether epoch's key may be destroyed (spec section 10.1): no current record
// version still uses it, and a full retention window has passed since its successor began, by which
// time every trash and history version written under it has aged out. With a zero window - Shred
// now - it retires as soon as the successor exists.
func EpochRetired(epoch uint32, rotations []Rotation, inUse func(uint32) bool, retentionDays uint16, nowUnixMilli uint64) bool {
	if inUse(epoch) {
		return false
	}
	for _, r := range rotations {
		if r.Epoch == epoch+1 {
			return elapsed(r.StartedUnixMilli, nowUnixMilli, windowMillis(retentionDays))
		}
	}
	return false
}

// PurgeDue reports whether a tombstone has outlived the trash window.
func PurgeDue(deletedUnixMilli uint64, retentionDays uint16, nowUnixMilli uint64) bool {
	return elapsed(deletedUnixMilli, nowUnixMilli, windowMillis(retentionDays))
}

// RotationDue reports a periodic rotation: every half window, for a vault that changed since the
// last one. A zero window turns periodic rotation off; such a vault rotates only on removal,
// revocation and Shred now, or it would rotate on every sync.
func RotationDue(lastRotationUnixMilli uint64, changedSince bool, retentionDays uint16, nowUnixMilli uint64) bool {
	if !changedSince || retentionDays == 0 {
		return false
	}
	return elapsed(lastRotationUnixMilli, nowUnixMilli, windowMillis(retentionDays)/2)
}
