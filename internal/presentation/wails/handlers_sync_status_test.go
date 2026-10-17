package wails

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/usecase"
)

func TestAStoppedAccountSaysWhatItNeeds(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		reason string
		says   string
	}{
		{"keys changed", fmt.Errorf("pull: %w", synctrust.ErrKeysRotated), "signIn", "Sign in again"},
		{"removed", fmt.Errorf("device challenge: %w: %w", usecase.ErrSyncDeviceRevoked, synctrust.ErrRemoteUnauthorized), "revoked", "removed"},
		{"tampering", fmt.Errorf("%w: membership", synctrust.ErrIntegrity), "integrity", "failed verification"},
		// Both at once: what the person can do about it outranks the alarm it would otherwise raise.
		{"removed and unverifiable", errors.Join(synctrust.ErrIntegrity, usecase.ErrSyncDeviceRevoked), "revoked", "removed"},
		{"keys changed and unverifiable", errors.Join(synctrust.ErrIntegrity, synctrust.ErrKeysRotated), "signIn", "Sign in again"},
	} {
		dto := syncStatusDTO(usecase.SyncStatus{State: usecase.SyncStopped, Err: tc.err}, true)
		if dto.StopReason != tc.reason || !strings.Contains(dto.Message, tc.says) {
			t.Errorf("%s: reason %q, message %q; want %q and a message saying %q", tc.name, dto.StopReason, dto.Message, tc.reason, tc.says)
		}
		if strings.Contains(dto.Message, "failed verification") && tc.reason != "integrity" {
			t.Errorf("%s: %q; a routine event on another device must not read as tampering", tc.name, dto.Message)
		}
	}
}

func TestOnlyAStoppedAccountHasAStopReason(t *testing.T) {
	dto := syncStatusDTO(usecase.SyncStatus{State: usecase.SyncFailed, Err: errors.New("offline")}, true)
	if dto.StopReason != "" {
		t.Fatalf("stop reason %q on a round that will be retried", dto.StopReason)
	}
}

func TestTheStatusCarriesWhatTheRoundApplied(t *testing.T) {
	dto := syncStatusDTO(usecase.SyncStatus{State: usecase.SyncIdle, Report: usecase.SyncReport{Pull: usecase.PullReport{Applied: 2}}}, true)
	if dto.Applied != 2 {
		t.Fatalf("applied = %d; the frontend re-reads the vault only when told something arrived", dto.Applied)
	}
}

func TestTheStatusCarriesNewDevicesAndTheAccountFingerprint(t *testing.T) {
	a, f, _ := syncAPI()
	identity := make([]byte, 32)
	identity[0] = 7
	f.info = &usecase.SyncAccountInfo{ServerURL: "https://s", Username: "alice", ServerIdentity: make([]byte, 32), AccountIdentity: identity}
	f.newDevices = []usecase.NewDeviceNotice{{Device: vaultsync.ID{0xab, 0xab, 0xab, 0xab, 0xab, 0xab, 0xab, 0xab, 0xab, 0xab, 0xab, 0xab, 0xab, 0xab, 0xab, 0xab}, Name: "tablet", Platform: "android"}}
	st, err := a.GetSyncStatus()
	if err != nil {
		t.Fatal(err)
	}
	if len(st.NewDevices) != 1 || st.NewDevices[0].Name != "tablet" || st.NewDevices[0].ID != strings.Repeat("ab", 16) {
		t.Fatalf("new devices = %+v; a device that joined is shown until acknowledged", st.NewDevices)
	}
	if st.AccountFingerprint == "" || st.AccountFingerprint == st.Fingerprint {
		t.Fatalf("account fingerprint %q, server %q; the account's own fingerprint is what a team admin compares",
			st.AccountFingerprint, st.Fingerprint)
	}
}

func TestAcknowledgingDevicesValidatesEveryID(t *testing.T) {
	a, f, _ := syncAPI()
	good := strings.Repeat("ab", 16)
	for name, ids := range map[string][]string{
		"none":     nil,
		"not hex":  {good, "zz"},
		"too long": {good + "00"},
		"zero":     {strings.Repeat("0", 32)},
	} {
		if err := a.AcknowledgeSyncDevices(ids); !errors.Is(err, errInvalidSyncRequest) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if f.acknowledged != nil {
		t.Fatal("an invalid request reached the controller")
	}
	if err := a.AcknowledgeSyncDevices([]string{good}); err != nil || len(f.acknowledged) != 1 || f.acknowledged[0][0] != 0xab {
		t.Fatalf("acknowledge: %v, %v", err, f.acknowledged)
	}
}

func TestAConflictOverAReferenceShowsNoID(t *testing.T) {
	a, _, c := syncAPI()
	c.list = []domain.SyncConflict{{ID: "r", RecordType: "connection", ItemKey: "c1", Path: []string{"users", "[id=u-1]", "passAuth", "passwordId"},
		Local: []byte(`"5bba7c56-1a9b-4617"`), Remote: []byte(`"e3c1b10f-2d3a-44e3"`)}}
	list, err := a.ListSyncConflicts()
	if err != nil || len(list) != 1 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	if got := list[0]; !got.Secret || got.Local != "" || got.Remote != "" || got.ItemName != "named c1" || len(got.PathNames) != 4 {
		t.Fatalf("conflict = %+v; a saved-password reference is an internal id, not something to show", got)
	}
}
