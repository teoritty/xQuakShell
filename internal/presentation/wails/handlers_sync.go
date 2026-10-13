package wails

import (
	"context"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/url"
	"strings"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/usecase"
)

// EventSyncStatus carries every status change of the sync engine to the indicator.
const EventSyncStatus = "SyncStatus"

// errSyncUnavailable is what every handler returns when sync is not wired.
var errSyncUnavailable = errors.New("sync unavailable")

// errInvalidSyncRequest covers every rejected argument, in one message for the same reason the
// local terminal handlers use one: the caller is code in a WebView.
var errInvalidSyncRequest = errors.New("invalid sync request")

// SyncCommands is the slice of the sync controller this layer needs.
type SyncCommands interface {
	Start(ctx context.Context) error
	Stop()
	Status() (usecase.SyncStatus, bool)
	AccountInfo() (usecase.SyncAccountInfo, bool, error)
	Probe(ctx context.Context, url string) (usecase.ServerProbe, error)
	Register(ctx context.Context, cred usecase.SyncCredentials) error
	Login(ctx context.Context, cred usecase.SyncCredentials) (bool, error)
	Join(ctx context.Context, choice usecase.JoinChoice) (string, error)
	Relogin(ctx context.Context, password []byte) error
	ChangePassword(ctx context.Context, old, next []byte) error
	ResetPassword(ctx context.Context, next []byte) error
	RevokeDevices(ctx context.Context, devices []vaultsync.ID, password []byte) error
	Disable(ctx context.Context) error
	SyncNow() error
	Devices(ctx context.Context) ([]usecase.DeviceInfo, error)
	HeldChanges(ctx context.Context) ([]usecase.HeldChange, error)
	ConfirmHeld(ctx context.Context, record vaultsync.ID, masterPassword string) error
	RejectHeld(ctx context.Context, record vaultsync.ID) error
}

// SyncConflictCommands lists and resolves credential conflicts.
type SyncConflictCommands interface {
	List() ([]domain.SyncConflict, error)
	Resolve(ctx context.Context, id string, useOther bool) error
}

// SetSyncServices wires sync. A setter for the reason SetLocalTerminalService gives.
func (a *AppAPI) SetSyncServices(sync SyncCommands, conflicts SyncConflictCommands) {
	a.sync, a.syncConflicts = sync, conflicts
}

// EmitSyncStatus is the controller's status callback: it forwards each change to the indicator.
func (a *AppAPI) EmitSyncStatus(st usecase.SyncStatus) {
	if a.ctx == nil {
		return
	}
	wailsrt.EventsEmit(a.ctx, EventSyncStatus, syncStatusDTO(st, true))
}

// startSync resumes sync after the vault opened. A failure is logged, never fatal: the vault must
// open whether or not the sync server is reachable or the stored account is sound.
func (a *AppAPI) startSync() {
	if a.sync == nil {
		return
	}
	if err := a.sync.Start(a.reqCtx()); err != nil {
		slog.Warn("sync did not start", "component", "sync", "err", err)
	}
}

// stopSync runs as the vault locks: the keys are about to leave memory.
func (a *AppAPI) stopSync() {
	if a.sync != nil {
		a.sync.Stop()
	}
}

// GetSyncStatus reports whether sync is enabled and running, and the account it syncs.
func (a *AppAPI) GetSyncStatus() (SyncStatusDTO, error) {
	if a.sync == nil {
		return SyncStatusDTO{State: "off"}, nil
	}
	st, running := a.sync.Status()
	dto := syncStatusDTO(st, running)
	info, ok, err := a.sync.AccountInfo()
	if err != nil {
		return SyncStatusDTO{}, userSyncError(err)
	}
	if ok {
		dto.Enabled, dto.ServerURL, dto.Username, dto.RekeyPending = true, info.ServerURL, info.Username, info.RekeyPending
		dto.Fingerprint = vaultsync.AccountFingerprint(info.ServerIdentity)
		dto.JoinPending = info.JoinPending
	}
	return dto, nil
}

// ProbeSyncServer fetches a server's identity on first contact; nothing else is sent until the
// user confirmed its fingerprint and EnableSync is called with that key.
func (a *AppAPI) ProbeSyncServer(serverURL string) (SyncServerProbeDTO, error) {
	if a.sync == nil {
		return SyncServerProbeDTO{}, errSyncUnavailable
	}
	if !validSyncURL(serverURL) {
		return SyncServerProbeDTO{}, errInvalidSyncRequest
	}
	p, err := a.sync.Probe(a.reqCtx(), serverURL)
	if err != nil {
		return SyncServerProbeDTO{}, userSyncError(err)
	}
	return SyncServerProbeDTO{IdentityKey: hex.EncodeToString(p.IdentityKey), Fingerprint: p.Fingerprint}, nil
}

// EnableSync registers an account or logs in to one, pinning the identity key the user confirmed.
func (a *AppAPI) EnableSync(req EnableSyncRequest) (EnableSyncResultDTO, error) {
	if a.sync == nil {
		return EnableSyncResultDTO{}, errSyncUnavailable
	}
	pin, err := hex.DecodeString(req.IdentityKey)
	if err != nil || len(pin) != 32 || !validSyncURL(req.ServerURL) || !validSyncField(req.Username) || req.Password == "" ||
		(req.Register && !validSyncField(req.Invite)) {
		return EnableSyncResultDTO{}, errInvalidSyncRequest
	}
	cred := usecase.SyncCredentials{ServerURL: req.ServerURL, Pin: pin, Username: req.Username, Password: []byte(req.Password), Invite: req.Invite}
	if req.Register {
		return EnableSyncResultDTO{}, userSyncError(a.sync.Register(a.reqCtx(), cred))
	}
	need, err := a.sync.Login(a.reqCtx(), cred)
	return EnableSyncResultDTO{JoinNeeded: need}, userSyncError(err)
}

// JoinSync finishes a login that found data on both sides: "merge" or "replace". It returns the
// name of the vault backup taken first.
func (a *AppAPI) JoinSync(choice string) (string, error) {
	if a.sync == nil {
		return "", errSyncUnavailable
	}
	var c usecase.JoinChoice
	switch choice {
	case "merge":
		c = usecase.JoinMerge
	case "replace":
		c = usecase.JoinReplace
	default:
		return "", errInvalidSyncRequest
	}
	name, err := a.sync.Join(a.reqCtx(), c)
	return name, userSyncError(err)
}

func validSyncURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && u.Scheme == "https" && u.Host != "" && len(s) <= 2048
}

func validSyncField(s string) bool {
	return strings.TrimSpace(s) != "" && len(s) <= 256
}
