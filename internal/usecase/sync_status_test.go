package usecase

import (
	"errors"
	"fmt"
	"testing"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

func rollupIDs() (personal, teamA, teamB vaultsync.ID) {
	personal[0], teamA[0], teamB[0] = 9, 1, 2
	return
}

func TestRollupShowsTheWorstVault(t *testing.T) {
	personal, team, _ := rollupIDs()
	broken := fmt.Errorf("%w: membership", synctrust.ErrIntegrity)
	per := map[vaultsync.ID]SyncStatus{
		personal: {Vault: personal, State: SyncStopped, Err: broken},
		team:     {Vault: team, State: SyncIdle},
	}
	got := rollupStatus(per, personal)
	if got.State != SyncStopped || !errors.Is(got.Err, synctrust.ErrIntegrity) {
		t.Fatalf("rollup = %v / %v; a team vault that synced must not hide the stopped personal vault", got.State, got.Err)
	}
}

func TestRollupAddsUpWhatWaits(t *testing.T) {
	personal, team, _ := rollupIDs()
	per := map[vaultsync.ID]SyncStatus{
		personal: {State: SyncIdle, Report: SyncReport{Pull: PullReport{Held: 1, Quarantined: 2}}},
		team:     {State: SyncIdle, Report: SyncReport{Pull: PullReport{Held: 3, Quarantined: 4}}},
	}
	if got := rollupStatus(per, personal); got.Report.Pull.Held != 4 || got.Report.Pull.Quarantined != 6 {
		t.Fatalf("held %d, quarantined %d; every vault's waiting changes count", got.Report.Pull.Held, got.Report.Pull.Quarantined)
	}
}

func TestRollupPrefersTheAccountLevelReason(t *testing.T) {
	personal, team, _ := rollupIDs()
	for _, reason := range []error{synctrust.ErrKeysRotated, ErrSyncDeviceRevoked} {
		per := map[vaultsync.ID]SyncStatus{
			personal: {State: SyncStopped, Err: fmt.Errorf("%w: device log", synctrust.ErrIntegrity)},
			team:     {State: SyncStopped, Err: fmt.Errorf("pull: %w", reason)},
		}
		if got := rollupStatus(per, personal); !errors.Is(got.Err, reason) {
			t.Fatalf("rollup error = %v; %v is about the account and decides what the person does next", got.Err, reason)
		}
	}
}

func TestRollupPicksTheSameMessageEveryTime(t *testing.T) {
	personal, a, b := rollupIDs()
	errA, errB := errors.New("a failed"), errors.New("b failed")
	per := map[vaultsync.ID]SyncStatus{
		b: {State: SyncFailed, Err: errB},
		a: {State: SyncFailed, Err: errA},
	}
	for range 20 {
		if got := rollupStatus(per, personal); !errors.Is(got.Err, errA) {
			t.Fatalf("rollup error = %v; equal states resolve in id order, not map order", got.Err)
		}
	}
	per[personal] = SyncStatus{State: SyncFailed, Err: errors.New("personal failed")}
	if got := rollupStatus(per, personal); got.Err.Error() != "personal failed" {
		t.Fatalf("rollup error = %v; the personal vault speaks first among equals", got.Err)
	}
}

func TestControllerReportsTheAccountNotTheLastVault(t *testing.T) {
	personal, team, _ := rollupIDs()
	var seen []SyncStatus
	c := NewSyncController(SyncControllerDeps{OnStatus: func(st SyncStatus) { seen = append(seen, st) }})
	c.session.Personal = personal
	c.record(SyncStatus{Vault: personal, State: SyncStopped, Err: synctrust.ErrKeysRotated})
	c.record(SyncStatus{Vault: team, State: SyncIdle, Report: SyncReport{Pull: PullReport{Applied: 3}}})
	last := seen[len(seen)-1]
	if last.State != SyncStopped || !errors.Is(last.Err, synctrust.ErrKeysRotated) {
		t.Fatalf("reported %v / %v after a team round; the account is still stopped", last.State, last.Err)
	}
	if last.Report.Pull.Applied != 3 {
		t.Fatalf("applied = %d; the reporting round's changes tell the views to re-read", last.Report.Pull.Applied)
	}
	if st, _ := c.Status(); st.State != SyncStopped {
		t.Fatalf("Status() = %v; it answers for the account", st.State)
	}
}

// A team the account left must not keep the account stopped, nor be counted, after it is gone.
func TestALeftVaultStopsCountingTowardTheAccount(t *testing.T) {
	personal, team, _ := rollupIDs()
	var seen []SyncStatus
	c := NewSyncController(SyncControllerDeps{OnStatus: func(st SyncStatus) { seen = append(seen, st) }})
	c.session.Personal = personal
	both := []vaultsync.ID{personal, team}
	c.record(SyncStatus{Vault: team, State: SyncStopped, Err: synctrust.ErrIntegrity, Vaults: both})
	c.record(SyncStatus{Vault: personal, State: SyncIdle, Vaults: both})
	if last := seen[len(seen)-1]; last.State != SyncStopped || last.VaultsChanged {
		t.Fatalf("state %v, changed %v; the first pass sets the vaults without calling it a change", last.State, last.VaultsChanged)
	}
	c.record(SyncStatus{Vault: personal, State: SyncIdle, Vaults: []vaultsync.ID{personal}})
	last := seen[len(seen)-1]
	if last.State != SyncIdle || last.Err != nil {
		t.Fatalf("state %v / %v; a left team's old failure no longer speaks for the account", last.State, last.Err)
	}
	if !last.VaultsChanged {
		t.Fatal("leaving a team is a change the views must show")
	}
	c.record(SyncStatus{Vault: personal, State: SyncIdle, Vaults: []vaultsync.ID{personal}})
	if seen[len(seen)-1].VaultsChanged {
		t.Fatal("an unchanged set of vaults is reported as changed")
	}
}

// A role change or a removal is a membership op with no record, so nothing is applied; the views
// learn of it only from the membership position moving, or a viewer keeps an editable form.
func TestAMembershipChangeIsAChangeTheViewsMustShow(t *testing.T) {
	personal, team, _ := rollupIDs()
	var seen []SyncStatus
	c := NewSyncController(SyncControllerDeps{OnStatus: func(st SyncStatus) { seen = append(seen, st) }})
	at := func(seq int, state SyncState) SyncStatus {
		return SyncStatus{Vault: team, State: state, Report: SyncReport{Pull: PullReport{MembershipSeq: seq}}}
	}
	c.record(SyncStatus{Vault: personal, State: SyncIdle})
	c.record(at(3, SyncIdle))
	if seen[len(seen)-1].VaultsChanged {
		t.Fatal("the first position seen is where the views already are, not a change")
	}
	c.record(at(4, SyncIdle))
	if !seen[len(seen)-1].VaultsChanged {
		t.Fatal("the membership moved and the views were not told")
	}
	c.record(at(4, SyncIdle))
	if seen[len(seen)-1].VaultsChanged {
		t.Fatal("an unmoved membership is reported as changed")
	}
	c.record(at(0, SyncFailed))
	c.record(at(4, SyncIdle))
	for _, st := range seen[len(seen)-2:] {
		if st.VaultsChanged {
			t.Fatal("a failed round knows no position; it must not read as the membership moving")
		}
	}
}

// A joined team reaches the views when its first round is done: announced as it first appears,
// before it was pulled, the views re-read a vault that did not hold it yet and were never told again.
func TestAJoinedTeamIsAChangeOnceItSynced(t *testing.T) {
	personal, team, _ := rollupIDs()
	var seen []SyncStatus
	c := NewSyncController(SyncControllerDeps{OnStatus: func(st SyncStatus) { seen = append(seen, st) }})
	both := []vaultsync.ID{personal, team}
	c.record(SyncStatus{Vault: personal, State: SyncIdle, Vaults: []vaultsync.ID{personal}})
	c.record(SyncStatus{Vault: personal, State: SyncRunning, Vaults: both})
	c.record(SyncStatus{Vault: personal, State: SyncIdle, Vaults: both})
	c.record(SyncStatus{Vault: team, State: SyncRunning, Vaults: both})
	for i, st := range seen {
		if st.VaultsChanged {
			t.Fatalf("report %d says changed before the new team synced", i)
		}
	}
	c.record(SyncStatus{Vault: team, State: SyncIdle, Vaults: both})
	if !seen[len(seen)-1].VaultsChanged {
		t.Fatal("a team joined with nothing to apply must still reach the views once it synced")
	}
	c.record(SyncStatus{Vault: team, State: SyncIdle, Vaults: both})
	if seen[len(seen)-1].VaultsChanged {
		t.Fatal("a team is announced once, not on every round")
	}
}
