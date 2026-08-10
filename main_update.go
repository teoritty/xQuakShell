package main

import (
	"xquakshell/internal/domain"
	infragithub "xquakshell/internal/infra/github"
	"xquakshell/internal/presentation/wails"
	"xquakshell/internal/usecase"
)

// updateRepoOwner and updateRepoName name the repository whose releases this build compares itself
// against. They are spelled out here, in the composition root, rather than inside the adapter: a
// fork that does not change them would be telling its users to install the upstream project.
const (
	updateRepoOwner = "teoritty"
	updateRepoName  = "xQuakShell"
)

// newUpdateService wires the release check that makes the support policy honest: only the latest
// release is supported (ADR-017), and a portable archive on a disk has no other way of finding out
// that it is not the latest. It downloads and installs nothing.
//
// The version it compares against is wails.AppVersion — the same value the About panel shows and
// the release workflow stamps at link time — so a dev build reports the fallback version and simply
// never finds itself out of date.
func newUpdateService(api *wails.AppAPI, auditLogRepo domain.AuditLogRepository) *usecase.UpdateService {
	feed := infragithub.NewReleaseFeed(infragithub.NewClient(), updateRepoOwner, updateRepoName)
	return usecase.NewUpdateService(feed, api.SettingsService(), auditLogRepo, wails.AppVersion)
}
