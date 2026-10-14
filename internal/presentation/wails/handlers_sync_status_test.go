package wails

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"xquakshell/internal/domain/synctrust"
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
