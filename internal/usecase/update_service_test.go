package usecase

import (
	"context"
	"errors"
	"testing"

	"xquakshell/internal/domain"
)

type stubFeed struct {
	release domain.ReleaseInfo
	err     error
	calls   int
}

func (f *stubFeed) LatestStableRelease(_ context.Context) (domain.ReleaseInfo, error) {
	f.calls++
	return f.release, f.err
}

type stubVault struct {
	domain.VaultRepository
	data *domain.VaultData
	err  error
}

func (v *stubVault) GetData() (*domain.VaultData, error) {
	if v.err != nil {
		return nil, v.err
	}
	return v.data, nil
}

type recordingAudit struct {
	domain.AuditLogRepository
	entries []domain.AuditEntry
}

func (a *recordingAudit) Append(_ context.Context, entry domain.AuditEntry) error {
	a.entries = append(a.entries, entry)
	return nil
}

func updateFixture(t *testing.T, feed *stubFeed, configure func(*domain.AppSettings)) (*UpdateService, *recordingAudit) {
	t.Helper()
	data := domain.NewVaultData()
	if configure != nil {
		configure(data.Settings)
	}
	audit := &recordingAudit{}
	svc := NewUpdateService(feed, NewSettingsService(&stubVault{data: data}, nil, nil), audit, "1.0.0")
	return svc, audit
}

func TestUpdateCheckReportsANewerRelease(t *testing.T) {
	feed := &stubFeed{release: domain.ReleaseInfo{Version: "1.0.1", URL: "https://example.test/r/1.0.1"}}
	svc, _ := updateFixture(t, feed, nil)

	status := svc.Check(context.Background())
	if !status.UpdateAvailable {
		t.Error("1.0.1 must be offered to a build running 1.0.0")
	}
	if status.LatestVersion != "1.0.1" || status.ReleaseURL == "" {
		t.Errorf("status does not carry what the banner needs: %+v", status)
	}
	if !status.Checked {
		t.Error("a completed check must be marked as one, or the UI cannot tell it from never having looked")
	}
	if got := svc.Last(); got != status {
		t.Errorf("Last() = %+v, want the status just produced", got)
	}
}

func TestUpdateCheckIsSilentWhenAlreadyCurrent(t *testing.T) {
	feed := &stubFeed{release: domain.ReleaseInfo{Version: "1.0.0", URL: "https://example.test/r/1.0.0"}}
	svc, _ := updateFixture(t, feed, nil)

	status := svc.Check(context.Background())
	if status.UpdateAvailable {
		t.Error("the running version is the latest; nothing should be offered")
	}
	if !status.Checked {
		t.Error("being up to date is a result, not an absence of one")
	}
}

// Turning the check off must stop the request, not merely hide the banner. A setting that still
// contacts GitHub is not the setting the user was offered.
func TestUpdateCheckMakesNoRequestWhenTurnedOff(t *testing.T) {
	feed := &stubFeed{release: domain.ReleaseInfo{Version: "9.9.9"}}
	off := false
	svc, audit := updateFixture(t, feed, func(s *domain.AppSettings) {
		s.Updates.CheckOnStartup = &off
	})

	status := svc.Check(context.Background())
	if feed.calls != 0 {
		t.Errorf("the release feed was contacted %d times with the check turned off", feed.calls)
	}
	if status.Checked || status.UpdateAvailable {
		t.Errorf("a check that never ran must not report a result: %+v", status)
	}
	if len(audit.entries) != 0 {
		t.Error("nothing happened, so nothing should be audited")
	}
}

// The settings live in the vault, so a locked vault cannot answer whether the user allowed this.
// Failing closed is the only safe reading.
func TestUpdateCheckMakesNoRequestWhileTheVaultIsLocked(t *testing.T) {
	feed := &stubFeed{release: domain.ReleaseInfo{Version: "9.9.9"}}
	svc := NewUpdateService(feed, NewSettingsService(&stubVault{err: domain.ErrVaultLocked}, nil, nil), nil, "1.0.0")

	if status := svc.Check(context.Background()); status.Checked {
		t.Error("a locked vault must not produce a check result")
	}
	if feed.calls != 0 {
		t.Errorf("the release feed was contacted %d times while the vault was locked", feed.calls)
	}
}

func TestUpdateCheckSurvivesAnUnreachableFeed(t *testing.T) {
	feed := &stubFeed{err: errors.New("dial tcp: no route to host")}
	svc, _ := updateFixture(t, feed, nil)

	status := svc.Check(context.Background())
	if status.UpdateAvailable {
		t.Error("an unreachable feed must never claim an update exists")
	}
	if status.CurrentVersion != "1.0.0" {
		t.Errorf("the running version must survive a failed check, got %q", status.CurrentVersion)
	}
}

// An outbound connection the application makes on its own behalf is exactly what an audit log is
// for. It carries versions and nothing else.
func TestUpdateCheckIsAudited(t *testing.T) {
	feed := &stubFeed{release: domain.ReleaseInfo{Version: "1.0.1", URL: "https://example.test/r"}}
	svc, audit := updateFixture(t, feed, func(s *domain.AppSettings) {
		s.AuditLog.Enabled = true // off by default, so the opposite case is the interesting one below
	})

	svc.Check(context.Background())
	if len(audit.entries) != 1 {
		t.Fatalf("got %d audit entries, want 1", len(audit.entries))
	}
	entry := audit.entries[0]
	if entry.Category != domain.AuditCategorySystem {
		t.Errorf("category = %q, want %q", entry.Category, domain.AuditCategorySystem)
	}
	if entry.SessionID != "" || entry.ConnectionID != "" {
		t.Error("the check belongs to no session or connection")
	}
}

func TestUpdateCheckIsNotAuditedWhenAuditingIsOff(t *testing.T) {
	feed := &stubFeed{release: domain.ReleaseInfo{Version: "1.0.1"}}
	svc, audit := updateFixture(t, feed, func(s *domain.AppSettings) {
		s.AuditLog.Enabled = false
	})

	svc.Check(context.Background())
	if len(audit.entries) != 0 {
		t.Errorf("audit logging is off, got %d entries", len(audit.entries))
	}
}
