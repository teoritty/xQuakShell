package usecase

import (
	"testing"

	"xquakshell/internal/domain"
)

// MaxConcurrent becomes the transfer slot count directly, so a large value is that many concurrent
// SFTP operations with their goroutines and buffers. It carried a floor and no ceiling, unlike
// every other numeric setting normalizeSettings touches.
//
// The settings dialog constrains its input to 1..16, but an HTML attribute constrains the dialog,
// not the RPC behind it — anything reaching the Wails bridge writes whatever it likes.
func TestNormalizeSettingsCapsConcurrentTransfers(t *testing.T) {
	for _, requested := range []int{17, 100, 1_000_000} {
		got := normalizeSettings(domain.AppSettings{
			Transfer: domain.TransferSettings{MaxConcurrent: requested, ConnectionTimeoutSec: 15},
		})
		if got.Transfer.MaxConcurrent != maxConcurrentTransfers {
			t.Errorf("MaxConcurrent %d normalized to %d, want the ceiling %d",
				requested, got.Transfer.MaxConcurrent, maxConcurrentTransfers)
		}
	}
}

// A stuck transfer holding a slot for a day is the shape this bounds.
func TestNormalizeSettingsCapsTheConnectionTimeout(t *testing.T) {
	got := normalizeSettings(domain.AppSettings{
		Transfer: domain.TransferSettings{MaxConcurrent: 4, ConnectionTimeoutSec: 86400},
	})
	if got.Transfer.ConnectionTimeoutSec != maxTransferConnectionTimeoutSec {
		t.Errorf("ConnectionTimeoutSec 86400 normalized to %d, want the ceiling %d",
			got.Transfer.ConnectionTimeoutSec, maxTransferConnectionTimeoutSec)
	}
}

// The floor is what the ceiling was added beside, and it still has to work: zero and negative are
// "unset", not "unlimited".
func TestNormalizeSettingsStillFillsTheFloor(t *testing.T) {
	got := normalizeSettings(domain.AppSettings{
		Transfer: domain.TransferSettings{MaxConcurrent: 0, ConnectionTimeoutSec: -1},
	})
	if got.Transfer.MaxConcurrent != 4 {
		t.Errorf("MaxConcurrent 0 normalized to %d, want the default 4", got.Transfer.MaxConcurrent)
	}
	if got.Transfer.ConnectionTimeoutSec != 15 {
		t.Errorf("ConnectionTimeoutSec -1 normalized to %d, want the default 15", got.Transfer.ConnectionTimeoutSec)
	}
}

// Everything the dialog can produce must survive untouched, or the clamp silently rewrites a
// setting the user chose.
func TestNormalizeSettingsLeavesEveryDialogValueAlone(t *testing.T) {
	for concurrent := 1; concurrent <= maxConcurrentTransfers; concurrent++ {
		got := normalizeSettings(domain.AppSettings{
			Transfer: domain.TransferSettings{MaxConcurrent: concurrent, ConnectionTimeoutSec: 15},
		})
		if got.Transfer.MaxConcurrent != concurrent {
			t.Errorf("MaxConcurrent %d was rewritten to %d", concurrent, got.Transfer.MaxConcurrent)
		}
	}
	for _, timeout := range []int{5, 15, 60, maxTransferConnectionTimeoutSec} {
		got := normalizeSettings(domain.AppSettings{
			Transfer: domain.TransferSettings{MaxConcurrent: 4, ConnectionTimeoutSec: timeout},
		})
		if got.Transfer.ConnectionTimeoutSec != timeout {
			t.Errorf("ConnectionTimeoutSec %d was rewritten to %d", timeout, got.Transfer.ConnectionTimeoutSec)
		}
	}
}

// The ceilings must match what the dialog offers, or the two disagree and a value the user can pick
// gets silently rewritten on save.
func TestTheCeilingsMatchTheDialogBounds(t *testing.T) {
	// frontend/src/lib/SettingsDialog.svelte: max="16" and max="300".
	if maxConcurrentTransfers != 16 {
		t.Errorf("maxConcurrentTransfers = %d, but the dialog offers up to 16", maxConcurrentTransfers)
	}
	if maxTransferConnectionTimeoutSec != 300 {
		t.Errorf("maxTransferConnectionTimeoutSec = %d, but the dialog offers up to 300", maxTransferConnectionTimeoutSec)
	}
}

// The language is a filename component on the way to <exe>/data/locales, so a value that never
// reached the dialog's dropdown must not reach the catalogue either. Normalizing on the way in
// means the stored setting is always something a path can safely be built from.
func TestNormalizeSettingsRejectsAMalformedLanguage(t *testing.T) {
	for _, requested := range []string{"", "../../etc", "en/../..", "ENGLISH", "e", "en_US"} {
		got := normalizeSettings(domain.AppSettings{Language: requested})
		if got.Language != domain.DefaultLocale {
			t.Errorf("Language %q normalized to %q, want the default %q",
				requested, got.Language, domain.DefaultLocale)
		}
	}
}

// A language this build has never heard of is not malformed, and a vault written by a newer build
// is a normal thing to open — it falls back to English rather than being refused.
func TestNormalizeSettingsKeepsAWellFormedLanguage(t *testing.T) {
	for _, requested := range []string{"ru", "de", "pt-BR"} {
		if got := normalizeSettings(domain.AppSettings{Language: requested}); got.Language != requested {
			t.Errorf("Language %q normalized to %q, want it kept", requested, got.Language)
		}
	}
}
