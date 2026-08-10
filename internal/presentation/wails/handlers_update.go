package wails

import (
	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	"xquakshell/internal/domain"
	"xquakshell/internal/pkg/safego"
	"xquakshell/internal/usecase"
)

// UpdateStatusDTO is what the banner renders.
type UpdateStatusDTO struct {
	CurrentVersion  string `json:"currentVersion"`
	LatestVersion   string `json:"latestVersion"`
	ReleaseURL      string `json:"releaseUrl"`
	UpdateAvailable bool   `json:"updateAvailable"`
	Checked         bool   `json:"checked"`
}

func updateStatusToDTO(s domain.UpdateStatus) UpdateStatusDTO {
	return UpdateStatusDTO{
		CurrentVersion:  s.CurrentVersion,
		LatestVersion:   s.LatestVersion,
		ReleaseURL:      s.ReleaseURL,
		UpdateAvailable: s.UpdateAvailable,
		Checked:         s.Checked,
	}
}

// SetUpdateService wires the update checker in after construction, like the other optional
// services: NewAppAPI's parameter list is long enough already, and an app built without a release
// feed simply never checks.
func (a *AppAPI) SetUpdateService(svc *usecase.UpdateService) {
	a.updateSvc = svc
}

// GetUpdateStatus returns the last known result without contacting anything, so a component that
// mounts after the check finished still renders the banner.
func (a *AppAPI) GetUpdateStatus() UpdateStatusDTO {
	if a.updateSvc == nil {
		return UpdateStatusDTO{}
	}
	return updateStatusToDTO(a.updateSvc.Last())
}

// startUpdateCheck runs one check in the background.
//
// It is triggered from the vault-opened path rather than at startup because the setting that
// permits the request lives inside the encrypted vault: before the vault is open the application
// cannot know whether the user allowed it, and making the request anyway would be making it
// without consent. In practice this is still once per run — the app is unusable until unlocked.
//
// It never surfaces an error. Being offline is the normal state of a great many installations of
// an SSH client, and a failed check is not something the user did or can fix.
func (a *AppAPI) startUpdateCheck() {
	if a.updateSvc == nil {
		return
	}
	ctx := a.reqCtx()
	safego.GoNamed("update.check", func() {
		status := a.updateSvc.Check(ctx)
		if !status.UpdateAvailable || a.ctx == nil {
			return
		}
		wailsrt.EventsEmit(a.ctx, EventUpdateAvailable, updateStatusToDTO(status))
	})
}
