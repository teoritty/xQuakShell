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
