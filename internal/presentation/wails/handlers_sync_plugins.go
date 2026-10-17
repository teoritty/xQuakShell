package wails

import (
	"context"
	"errors"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/usecase"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"
)

// EventSyncPluginData carries a plugin data report worth the user's attention: files edited both
// here and elsewhere, or files left out of sync (spec §6.5).
const EventSyncPluginData = "SyncPluginData"

// PluginOfferCommands is the slice of plugins-by-reference sync the Sync page needs (spec §6.4).
type PluginOfferCommands interface {
	Offers(ctx context.Context) ([]usecase.PluginOffer, error)
	PreviewOffer(ctx context.Context, pluginID string) (usecase.GitHubPluginPreviewDTO, error)
	InstallOffer(ctx context.Context, req usecase.OfferInstallRequest) error
	KeepPlugin(ctx context.Context, pluginID string) error
}

// PluginDataCommands is the slice of plugin data sync the Sync page needs (spec §6.5).
type PluginDataCommands interface {
	Status() ([]usecase.PluginDataStatus, error)
	SetSyncEnabled(ctx context.Context, pluginID string, enabled bool) error
}

// SetPluginSyncServices wires the plugin half of the Sync page. A setter for the reason
// SetLocalTerminalService gives.
func (a *AppAPI) SetPluginSyncServices(offers PluginOfferCommands, data PluginDataCommands) {
	a.pluginOffers, a.pluginData = offers, data
}

// SyncPluginOfferDTO is one plugin another device recorded that needs the user here.
type SyncPluginOfferDTO struct {
	PluginID string `json:"pluginId"`
	Version  string `json:"version"`
	// State is install, manual, mismatch, removed or update.
	State string `json:"state"`
	Repo  string `json:"repo,omitempty"`
	Tag   string `json:"tag,omitempty"`
}

// SyncPluginInstallRequest installs an offered plugin with the consents the dialog collected.
type SyncPluginInstallRequest struct {
	PluginID        string `json:"pluginId"`
	Secret          bool   `json:"secret"`
	Auth            bool   `json:"auth"`
	Tunnel          bool   `json:"tunnel"`
	MultiSession    bool   `json:"multiSession"`
	Network         bool   `json:"network"`
	Exec            bool   `json:"exec"`
	AcceptDifferent bool   `json:"acceptDifferent"`
}

// SyncPluginInstallResultDTO says how an offer install ended. "mismatch" is not an error: the
// release differs from the build the other device recorded, nothing was installed, and the dialog
// asks whether to install it anyway as a new version.
type SyncPluginInstallResultDTO struct {
	Status string `json:"status"`
}

// SyncPluginDataDTO is one plugin whose data follows the user.
type SyncPluginDataDTO struct {
	PluginID string `json:"pluginId"`
	Name     string `json:"name"`
	Enabled  bool   `json:"enabled"`
	Waiting  int    `json:"waiting"`
	Files    int    `json:"files"`
}

// SyncPluginDataReportDTO is the payload of EventSyncPluginData: file names only, never contents.
type SyncPluginDataReportDTO struct {
	PluginID  string   `json:"pluginId"`
	Conflicts []string `json:"conflicts"`
	Skipped   []string `json:"skipped"`
}

var pluginOfferMessages = []struct {
	err error
	msg string
}{
	{usecase.ErrPluginOfferManual, "This plugin was installed from a file on your other device; install it manually here."},
	{usecase.ErrPluginOfferNotFound, "That plugin is no longer offered by your other devices."},
	{domainplugin.ErrPluginNotFound, "That plugin is not installed on this device."},
	{domain.ErrVaultLocked, "Unlock the vault first."},
}

// pluginOfferError maps the offer sentinels to fixed sentences. Anything else comes from the GitHub
// install pipeline and is passed on as InstallGitHubPlugin passes it: the same consent, checksum and
// signature refusals the user sees when installing from a source.
func pluginOfferError(err error) error {
	for _, m := range pluginOfferMessages {
		if errors.Is(err, m.err) {
			return userFacingError{msg: m.msg, cause: err}
		}
	}
	return err
}

func validPluginID(id string) bool { return domainplugin.ValidateID(id) == nil }

// ListSyncPluginOffers lists the plugins from the user's other devices that need them here.
func (a *AppAPI) ListSyncPluginOffers() ([]SyncPluginOfferDTO, error) {
	if a.pluginOffers == nil {
		return nil, errSyncUnavailable
	}
	offers, err := a.pluginOffers.Offers(a.reqCtx())
	if err != nil {
		return nil, pluginOfferError(err)
	}
	out := make([]SyncPluginOfferDTO, 0, len(offers))
	for _, o := range offers {
		out = append(out, SyncPluginOfferDTO{PluginID: o.Plugin.ID, Version: o.Plugin.Version, State: string(o.State),
			Repo: o.Plugin.Source.Repo, Tag: o.Plugin.Source.Tag})
	}
	return out, nil
}

// PreviewSyncPluginOffer prepares the consent screen for an offered plugin's recorded release.
func (a *AppAPI) PreviewSyncPluginOffer(pluginID string) (GitHubPluginPreviewResponseDTO, error) {
	if a.pluginOffers == nil {
		return GitHubPluginPreviewResponseDTO{}, errSyncUnavailable
	}
	if !validPluginID(pluginID) {
		return GitHubPluginPreviewResponseDTO{}, errInvalidSyncRequest
	}
	p, err := a.pluginOffers.PreviewOffer(a.reqCtx(), pluginID)
	if err != nil {
		return GitHubPluginPreviewResponseDTO{}, pluginOfferError(err)
	}
	return githubPreviewToDTO(p), nil
}

// InstallSyncPluginOffer installs an offered plugin through the normal pipeline, held to the build
// the other device recorded.
func (a *AppAPI) InstallSyncPluginOffer(req SyncPluginInstallRequest) (SyncPluginInstallResultDTO, error) {
	if a.pluginOffers == nil {
		return SyncPluginInstallResultDTO{}, errSyncUnavailable
	}
	if !validPluginID(req.PluginID) {
		return SyncPluginInstallResultDTO{}, errInvalidSyncRequest
	}
	err := a.pluginOffers.InstallOffer(a.reqCtx(), usecase.OfferInstallRequest{PluginID: req.PluginID,
		AcceptDifferent: req.AcceptDifferent, Consents: domainplugin.InstallConsents{SecretAccess: req.Secret,
			AuthProvider: req.Auth, TunnelProvider: req.Tunnel, MultiSession: req.MultiSession,
			ArbitraryNetwork: req.Network, Exec: req.Exec}})
	switch {
	case errors.Is(err, usecase.ErrPluginDigestMismatch):
		return SyncPluginInstallResultDTO{Status: "mismatch"}, nil
	case err != nil:
		return SyncPluginInstallResultDTO{}, pluginOfferError(err)
	}
	a.EmitPluginContributionsChanged()
	return SyncPluginInstallResultDTO{Status: "installed"}, nil
}

// KeepSyncPlugin keeps the build installed here: a mismatching build becomes the version on every
// device, and a plugin another device uninstalled is recorded again and turned back on.
func (a *AppAPI) KeepSyncPlugin(pluginID string) error {
	if a.pluginOffers == nil {
		return errSyncUnavailable
	}
	if !validPluginID(pluginID) {
		return errInvalidSyncRequest
	}
	if err := a.pluginOffers.KeepPlugin(a.reqCtx(), pluginID); err != nil {
		return pluginOfferError(err)
	}
	a.EmitPluginContributionsChanged()
	return nil
}

// ListSyncPluginData lists the plugins whose data follows the user, with what waits for a restart.
func (a *AppAPI) ListSyncPluginData() ([]SyncPluginDataDTO, error) {
	if a.pluginData == nil {
		return nil, errSyncUnavailable
	}
	status, err := a.pluginData.Status()
	if err != nil {
		return nil, pluginOfferError(err)
	}
	out := make([]SyncPluginDataDTO, 0, len(status))
	for _, s := range status {
		out = append(out, SyncPluginDataDTO{PluginID: s.Plugin, Name: s.Name, Enabled: s.Enabled, Waiting: s.Waiting, Files: s.Files})
	}
	return out, nil
}

// SetSyncPluginData turns syncing of one plugin's data on or off on this device only.
func (a *AppAPI) SetSyncPluginData(pluginID string, enabled bool) error {
	if a.pluginData == nil {
		return errSyncUnavailable
	}
	if !validPluginID(pluginID) {
		return errInvalidSyncRequest
	}
	return pluginOfferError(a.pluginData.SetSyncEnabled(a.reqCtx(), pluginID, enabled))
}

// RestartSyncPlugin restarts a plugin so the data updates waiting for it are applied.
func (a *AppAPI) RestartSyncPlugin(pluginID string) error {
	if !validPluginID(pluginID) {
		return errInvalidSyncRequest
	}
	if a.plugins == nil {
		return errPluginManagerUnavailable
	}
	ctx, cancel := context.WithTimeout(a.reqCtx(), pluginCallTimeout)
	defer cancel()
	return pluginOfferError(a.plugins.RestartPlugin(ctx, pluginID))
}

func pluginDataReportWorthShowing(r usecase.PluginDataReport) bool {
	return len(r.Conflicts) > 0 || len(r.Skipped) > 0
}

func pluginDataReportPayload(r usecase.PluginDataReport) SyncPluginDataReportDTO {
	return SyncPluginDataReportDTO{PluginID: r.Plugin, Conflicts: r.Conflicts, Skipped: r.Skipped}
}

// EmitPluginDataReport is plugin data sync's report callback: it tells the user about a conflict or
// a file left out, and stays quiet about routine pushes and applies.
func (a *AppAPI) EmitPluginDataReport(r usecase.PluginDataReport) {
	if a.ctx == nil || !pluginDataReportWorthShowing(r) {
		return
	}
	wailsrt.EventsEmit(a.ctx, EventSyncPluginData, pluginDataReportPayload(r))
}
