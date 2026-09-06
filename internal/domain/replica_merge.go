package domain

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
)

// ErrScopeMismatch indicates a replica document built for a different plugin's scope.
var ErrScopeMismatch = errors.New("replica belongs to another scope")

// ReplicaMerge is the outcome of comparing an arriving replica against the local one.
type ReplicaMerge struct {
	// Order is how the two stood before merging, and is what a log line reports.
	Order ReplicaOrder
	// Result is what this device should become.
	Result ReplicaDocument
	// Conflicts names objects edited on both sides while apart. They are left at the local value;
	// there is nothing in either version to say which of two afternoons of work matters less.
	Conflicts []string
	// Removed names objects this device has that the newer remote does not. They are kept, not
	// deleted: deletion never arrives over the wire (I9), so this is a proposal for the user.
	Removed []string
}

// MergeReplica combines an arriving replica into the local one.
//
// Nothing is ever lost without being reported. A remote that is strictly ahead is taken whole, which
// is safe precisely because the version vector says it has already seen this device's edits. A
// divergence keeps both sides: additions from either are taken, and the one thing that cannot be
// merged without guessing - the same object edited on both sides - is reported and left alone.
func MergeReplica(local, remote ReplicaDocument) (ReplicaMerge, error) {
	if local.Scope != remote.Scope {
		return ReplicaMerge{}, fmt.Errorf("%w: local %q, arriving %q", ErrScopeMismatch, local.Scope, remote.Scope)
	}
	order := local.Version.Compare(remote.Version)
	merge := ReplicaMerge{Order: order, Result: local}
	if order == ReplicaSame || order == ReplicaAhead {
		// This device has everything the other does. The remote catches up on the next push.
		return merge, nil
	}

	merge.Result.Version = local.Version.Merge(remote.Version)
	merge.Result.Connections, merge.Conflicts = mergeByID(local.Connections, remote.Connections,
		func(c Connection) string { return c.ID }, order)
	folders, folderConflicts := mergeByID(local.Folders, remote.Folders,
		func(f ConnectionFolder) string { return f.ID }, order)
	merge.Result.Folders = folders
	merge.Conflicts = append(merge.Conflicts, folderConflicts...)
	slices.Sort(merge.Conflicts)

	merge.Result.Passwords = mergeMaps(local.Passwords, remote.Passwords)
	merge.Result.Identities = mergeMaps(local.Identities, remote.Identities)
	merge.Result.KeyBlobs = mergeMaps(local.KeyBlobs, remote.KeyBlobs)
	merge.Removed = missingFromRemote(local.Connections, remote.Connections,
		func(c Connection) string { return c.ID })
	return merge, nil
}

// mergeByID takes every object either side has. Where both have one, the newer side wins when there
// is a newer side, and a divergence keeps the local value and names the object as a conflict.
func mergeByID[T any](local, remote []T, id func(T) string, order ReplicaOrder) (merged []T, conflicts []string) {
	byID := make(map[string]T, len(local)+len(remote))
	for _, item := range local {
		byID[id(item)] = item
	}
	for _, item := range remote {
		key := id(item)
		mine, both := byID[key]
		if !both {
			byID[key] = item
			continue
		}
		if sameContent(mine, item) {
			continue
		}
		if order == ReplicaDiverged {
			conflicts = append(conflicts, key)
			continue
		}
		byID[key] = item
	}
	for _, key := range slices.Sorted(maps.Keys(byID)) {
		merged = append(merged, byID[key])
	}
	return merged, conflicts
}

// mergeMaps takes every entry either side has, preferring the arriving one where both hold the same
// key. A secret is either present or not; there is no half of one to choose between.
func mergeMaps[T any](local, remote map[string]T) map[string]T {
	if len(local) == 0 && len(remote) == 0 {
		return nil
	}
	merged := make(map[string]T, len(local)+len(remote))
	maps.Copy(merged, local)
	maps.Copy(merged, remote)
	return merged
}

// missingFromRemote names what this device has and the arriving replica does not.
func missingFromRemote[T any](local, remote []T, id func(T) string) []string {
	present := make(map[string]struct{}, len(remote))
	for _, item := range remote {
		present[id(item)] = struct{}{}
	}
	var missing []string
	for _, item := range local {
		if _, still := present[id(item)]; !still {
			missing = append(missing, id(item))
		}
	}
	slices.Sort(missing)
	return missing
}

// sameContent compares two objects by their serialized form.
//
// Structural equality is not available - connections carry slices and maps - and comparing field by
// field would need updating every time one is added, which is the shape that silently stops noticing
// a change. encoding/json sorts map keys, so the comparison is stable.
func sameContent[T any](first, second T) bool {
	a, errA := json.Marshal(first)
	b, errB := json.Marshal(second)
	if errA != nil || errB != nil {
		return false
	}
	return string(a) == string(b)
}
