package domain_test

import (
	"encoding/json"
	"testing"

	"xquakshell/internal/domain"
)

func vector(pairs map[string]uint64) domain.VersionVector {
	return domain.VersionVector(pairs)
}

// Two replicas that have seen the same history are the same, and a fresh pair of empty ones is the
// ordinary state of a device that has never synchronised.
func TestReplicasThatHaveSeenTheSameHistoryAreEqual(t *testing.T) {
	for _, tc := range []struct{ local, remote domain.VersionVector }{
		{nil, nil},
		{vector(map[string]uint64{}), nil},
		{vector(map[string]uint64{"a": 3}), vector(map[string]uint64{"a": 3})},
		// A device recorded at zero has contributed nothing, which is what being absent means.
		{vector(map[string]uint64{"a": 3, "b": 0}), vector(map[string]uint64{"a": 3})},
	} {
		if got := tc.local.Compare(tc.remote); got != domain.ReplicaSame {
			t.Errorf("Compare(%v, %v) = %v, want ReplicaSame", tc.local, tc.remote, got)
		}
	}
}

// Ahead and behind are the ordinary cases: one side has everything the other has, and more.
func TestOneSidedProgressIsAheadOrBehind(t *testing.T) {
	older := vector(map[string]uint64{"a": 1})
	newer := vector(map[string]uint64{"a": 2})
	wider := vector(map[string]uint64{"a": 1, "b": 1})

	if got := newer.Compare(older); got != domain.ReplicaAhead {
		t.Errorf("a higher counter compared %v, want ReplicaAhead", got)
	}
	if got := older.Compare(newer); got != domain.ReplicaBehind {
		t.Errorf("a lower counter compared %v, want ReplicaBehind", got)
	}
	if got := wider.Compare(older); got != domain.ReplicaAhead {
		t.Errorf("an extra device compared %v, want ReplicaAhead", got)
	}
	if got := older.Compare(wider); got != domain.ReplicaBehind {
		t.Errorf("a missing device compared %v, want ReplicaBehind", got)
	}
}

// The case a scalar counter cannot see, and the reason this is a vector.
//
// Two devices that both edited while apart are each ahead of the other on their own history. A
// single number would put one of them "newer" and quietly discard the other's work; the vector says
// they diverged, which is a thing to tell the user about rather than resolve behind their back.
func TestTwoDevicesThatBothEditedHaveDiverged(t *testing.T) {
	first := vector(map[string]uint64{"a": 2, "b": 1})
	second := vector(map[string]uint64{"a": 1, "b": 2})

	if got := first.Compare(second); got != domain.ReplicaDiverged {
		t.Errorf("Compare = %v, want ReplicaDiverged", got)
	}
	if got := second.Compare(first); got != domain.ReplicaDiverged {
		t.Errorf("divergence is not symmetric: %v", got)
	}
}

// The rollback check (V12). A hostile or merely broken server serving yesterday's payload is behind,
// and behind is refused - the core decides this from what is inside the ciphertext, so the server
// cannot talk its way out of it.
func TestAReplayedOlderReplicaIsBehind(t *testing.T) {
	current := vector(map[string]uint64{"a": 7, "b": 4})
	replayed := vector(map[string]uint64{"a": 5, "b": 4})

	if got := current.Compare(replayed); got != domain.ReplicaAhead {
		t.Fatalf("a replayed payload compared %v, want the local side to be ReplicaAhead", got)
	}
}

// Advancing records this device's own progress and leaves everyone else's alone.
func TestAdvanceMovesOnlyThisDevice(t *testing.T) {
	before := vector(map[string]uint64{"a": 2, "b": 5})

	after := before.Advance("a")

	if after["a"] != 3 {
		t.Errorf("this device is at %d, want 3", after["a"])
	}
	if after["b"] != 5 {
		t.Errorf("another device moved to %d", after["b"])
	}
	if before["a"] != 2 {
		t.Error("Advance edited the vector it was given instead of returning a new one")
	}
}

// A device that has never contributed starts at one, so its first write is visible rather than
// indistinguishable from never having written.
func TestAdvanceStartsANewDeviceAtOne(t *testing.T) {
	after := domain.VersionVector(nil).Advance("a")

	if after["a"] != 1 {
		t.Fatalf("a new device is at %d, want 1", after["a"])
	}
}

// Merging takes the highest seen for every device, which is what "I have now seen your history too"
// means. Taking anything less would make the next comparison claim a divergence that was just
// resolved.
func TestMergingTakesTheHighestSeenOfEach(t *testing.T) {
	merged := vector(map[string]uint64{"a": 2, "b": 1}).Merge(vector(map[string]uint64{"a": 1, "b": 5, "c": 1}))

	for device, want := range map[string]uint64{"a": 2, "b": 5, "c": 1} {
		if merged[device] != want {
			t.Errorf("device %s merged to %d, want %d", device, merged[device], want)
		}
	}
}

// The vector travels inside the ciphertext, so it has to survive the trip. A vector that decoded
// differently would turn an ordinary sync into a divergence.
func TestAVersionVectorSurvivesAJSONRoundTrip(t *testing.T) {
	original := vector(map[string]uint64{"a": 2, "b": 5})

	raw, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var decoded domain.VersionVector
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got := decoded.Compare(original); got != domain.ReplicaSame {
		t.Fatalf("the decoded vector compared %v to the original", got)
	}
}

// The ordering is rendered in a log line and a message to the user, so it has to name itself.
func TestTheOrderingNamesItself(t *testing.T) {
	for order, want := range map[domain.ReplicaOrder]string{
		domain.ReplicaSame:     "same",
		domain.ReplicaAhead:    "ahead",
		domain.ReplicaBehind:   "behind",
		domain.ReplicaDiverged: "diverged",
	} {
		if got := order.String(); got != want {
			t.Errorf("String() = %q, want %q", got, want)
		}
	}
}
