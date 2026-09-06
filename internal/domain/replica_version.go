package domain

import "maps"

// ReplicaOrder is how one replica stands against another.
type ReplicaOrder int

const (
	// ReplicaSame means both have seen exactly the same history.
	ReplicaSame ReplicaOrder = iota
	// ReplicaAhead means this replica has seen everything the other has, and more.
	ReplicaAhead
	// ReplicaBehind means the other has seen everything this one has, and more.
	ReplicaBehind
	// ReplicaDiverged means each has seen something the other has not.
	//
	// It is a thing to tell the user about rather than resolve behind their back: both sides hold
	// work, and picking one silently is how a synchronisation loses an afternoon of edits.
	ReplicaDiverged
)

// String names the ordering for a log line or a message to the user.
func (o ReplicaOrder) String() string {
	switch o {
	case ReplicaAhead:
		return "ahead"
	case ReplicaBehind:
		return "behind"
	case ReplicaDiverged:
		return "diverged"
	default:
		return "same"
	}
}

// VersionVector records how much of each device's history a replica has seen (ADR-022, T2).
//
// It is a vector rather than a counter because a counter cannot see a divergence. Two devices that
// both edited while apart are each ahead of the other on their own history, and a single number
// would put one of them "newer" and quietly discard the other's work.
//
// It travels inside the ciphertext, which is the point: the ordering is decided by the core from
// what it can read, and a hostile server serving yesterday's payload cannot talk its way out of
// being behind. A device absent from the vector has contributed nothing, which is the same thing as
// being recorded at zero.
type VersionVector map[string]uint64

// Compare reports how this replica stands against another.
func (v VersionVector) Compare(other VersionVector) ReplicaOrder {
	var ahead, behind bool
	for device := range mergedDevices(v, other) {
		switch mine, theirs := v[device], other[device]; {
		case mine > theirs:
			ahead = true
		case mine < theirs:
			behind = true
		}
	}
	switch {
	case ahead && behind:
		return ReplicaDiverged
	case ahead:
		return ReplicaAhead
	case behind:
		return ReplicaBehind
	default:
		return ReplicaSame
	}
}

// Advance records one more step of this device's own history, leaving every other device alone.
//
// It returns a new vector rather than editing this one: a replica's version is part of what has
// already been written down, and advancing it in place would rewrite history that other code is
// still comparing against.
func (v VersionVector) Advance(device string) VersionVector {
	next := make(VersionVector, len(v)+1)
	maps.Copy(next, v)
	next[device]++
	return next
}

// Merge takes the highest seen for every device, which is what "I have now seen your history too"
// means. Taking anything less would make the next comparison report a divergence that was just
// resolved.
func (v VersionVector) Merge(other VersionVector) VersionVector {
	merged := make(VersionVector, len(v)+len(other))
	maps.Copy(merged, v)
	for device, seen := range other {
		if seen > merged[device] {
			merged[device] = seen
		}
	}
	return merged
}

// mergedDevices yields every device either side knows about, so a device present in one and absent
// from the other is compared rather than skipped.
func mergedDevices(first, second VersionVector) map[string]struct{} {
	devices := make(map[string]struct{}, len(first)+len(second))
	for device := range first {
		devices[device] = struct{}{}
	}
	for device := range second {
		devices[device] = struct{}{}
	}
	return devices
}
