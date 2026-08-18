package main

import (
	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	infragithub "xquakshell/internal/infra/github"
	infragitlab "xquakshell/internal/infra/gitlab"
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

// updateRepoForge picks which platform's releases the update check reads.
//
// It is one constant because the two are not interchangeable at runtime: a build points at exactly
// one release stream, and offering a user the newer of two independently-tagged streams would have
// them bounce between them.
//
// It follows wherever releases are actually published, and from 1.2.1 that is GitLab. A build that
// checked the other platform would report itself up to date forever: the stream it reads simply
// stops gaining tags, and nothing anywhere says so.
const updateRepoForge = domainplugin.ForgeGitLab

// newUpdateService wires the release check that makes the support policy honest: only the latest
// release is supported (ADR-017), and a portable archive on a disk has no other way of finding out
// that it is not the latest. It downloads and installs nothing.
//
// The version it compares against is wails.AppVersion — the same value the About panel shows and
// the release workflow stamps at link time — so a dev build reports the fallback version and simply
// never finds itself out of date.
func newUpdateService(api *wails.AppAPI, auditLogRepo domain.AuditLogRepository) *usecase.UpdateService {
	return usecase.NewUpdateService(newReleaseFeed(), api.SettingsService(), auditLogRepo, wails.AppVersion)
}

// newReleaseFeed builds the feed for whichever forge updateRepoForge names.
func newReleaseFeed() domain.ReleaseFeed {
	if updateRepoForge == domainplugin.ForgeGitLab {
		return infragitlab.NewReleaseFeed(infragitlab.NewClient(), updateRepoOwner, updateRepoName)
	}
	return infragithub.NewReleaseFeed(infragithub.NewClient(), updateRepoOwner, updateRepoName)
}
