package wails

import (
	"encoding/hex"
	"errors"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/usecase"
)

// SyncStatusDTO is the sync indicator and the header of the settings page.
type SyncStatusDTO struct {
	Enabled      bool   `json:"enabled"`
	Running      bool   `json:"running"`
	State        string `json:"state"`
	Message      string `json:"message,omitempty"`
	Held         int    `json:"held"`
	Quarantined  int    `json:"quarantined"`
	ServerURL    string `json:"serverUrl,omitempty"`
	Username     string `json:"username,omitempty"`
	Fingerprint  string `json:"fingerprint,omitempty"`
	RekeyPending bool   `json:"rekeyPending,omitempty"`
	JoinPending  bool   `json:"joinPending,omitempty"`
	// NewDevices joined the account after this device and wait until the person acknowledges them.
	NewDevices []SyncDeviceDTO `json:"newDevices,omitempty"`
	// AccountFingerprint is this account's own fingerprint, for a team admin to compare.
	AccountFingerprint string `json:"accountFingerprint,omitempty"`
	// StopReason says what a stopped account needs: "signIn" after keys changed elsewhere,
	// "revoked" for a removed device, "integrity" for a verification failure.
	StopReason string `json:"stopReason,omitempty"`
	// Applied counts the changes the reporting round wrote into the vault; above zero, the open
	// views re-read it.
	Applied int `json:"applied,omitempty"`
}

// SyncServerProbeDTO is a server identity for the user to compare with what its admin printed.
type SyncServerProbeDTO struct {
	IdentityKey string `json:"identityKey"`
	Fingerprint string `json:"fingerprint"`
}

// EnableSyncRequest enables sync by registering (with an invite) or logging in.
type EnableSyncRequest struct {
	ServerURL   string `json:"serverUrl"`
	IdentityKey string `json:"identityKey"`
	Username    string `json:"username"`
	Password    string `json:"password"`
	Invite      string `json:"invite"`
	Register    bool   `json:"register"`
}

// EnableSyncResultDTO tells the frontend whether to ask how to join.
type EnableSyncResultDTO struct {
	JoinNeeded bool `json:"joinNeeded"`
}

// SyncDeviceDTO is one row of the devices panel; the id is what RevokeSyncDevice takes back.
type SyncDeviceDTO struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Platform         string `json:"platform"`
	CreatedUnixMilli int64  `json:"createdUnixMilli"`
	Revoked          bool   `json:"revoked"`
	Current          bool   `json:"current"`
}

// SyncHeldDTO is a change waiting for the master password, or one that was refused.
type SyncHeldDTO struct {
	Record      string `json:"record"`
	Type        string `json:"type"`
	Key         string `json:"key"`
	Reason      string `json:"reason"`
	Pending     bool   `json:"pending"`
	AtUnixMilli int64  `json:"atUnixMilli"`
}

// SyncTrashItemDTO is a deleted record in the trash. Type and Key are empty when its content no
// longer opens, which is also when it cannot be restored.
type SyncTrashItemDTO struct {
	Record           string `json:"record"`
	Type             string `json:"type"`
	Key              string `json:"key"`
	DeletedUnixMilli int64  `json:"deletedUnixMilli"`
	PurgeUnixMilli   int64  `json:"purgeUnixMilli"`
	Restorable       bool   `json:"restorable"`
}

// SyncConflictDTO is a credential conflict. Values of secret fields are not sent: the person picks
// a side by device and time, and a password never has to be on screen to do it.
type SyncConflictDTO struct {
	ID          string   `json:"id"`
	RecordType  string   `json:"recordType"`
	ItemKey     string   `json:"itemKey"`
	Path        []string `json:"path"`
	Secret      bool     `json:"secret"`
	Local       string   `json:"local,omitempty"`
	Remote      string   `json:"remote,omitempty"`
	AtUnixMilli int64    `json:"atUnixMilli"`
}

func syncStateName(s usecase.SyncState) string {
	switch s {
	case usecase.SyncRunning:
		return "running"
	case usecase.SyncFailed:
		return "failed"
	case usecase.SyncStopped:
		return "stopped"
	case usecase.SyncIdle:
		return "idle"
	}
	return "off"
}

func syncStatusDTO(st usecase.SyncStatus, running bool) SyncStatusDTO {
	dto := SyncStatusDTO{Running: running, State: syncStateName(st.State), Held: st.Report.Pull.Held,
		Quarantined: st.Report.Pull.Quarantined}
	if !running {
		dto.State = "off"
	}
	if st.Err != nil {
		dto.Message = userSyncError(st.Err).Error()
	}
	if dto.State == "stopped" {
		dto.StopReason = syncStopReason(st.Err)
	}
	dto.Applied = st.Report.Pull.Applied
	return dto
}

// withSyncAccount adds what the device knows about the account: where it syncs, both fingerprints,
// and the devices waiting to be acknowledged.
func withSyncAccount(dto SyncStatusDTO, info usecase.SyncAccountInfo, fresh []usecase.NewDeviceNotice) SyncStatusDTO {
	dto.Enabled, dto.ServerURL, dto.Username, dto.RekeyPending = true, info.ServerURL, info.Username, info.RekeyPending
	dto.Fingerprint = vaultsync.AccountFingerprint(info.ServerIdentity)
	if len(info.AccountIdentity) > 0 {
		dto.AccountFingerprint = vaultsync.AccountFingerprint(info.AccountIdentity)
	}
	dto.JoinPending = info.JoinPending
	for _, d := range fresh {
		dto.NewDevices = append(dto.NewDevices, SyncDeviceDTO{ID: hex.EncodeToString(d.Device[:]), Name: d.Name,
			Platform: d.Platform, CreatedUnixMilli: d.CreatedUnixMilli})
	}
	return dto
}

// syncStopReason names what a stopped account needs, in the order syncMessages matches them.
func syncStopReason(err error) string {
	switch {
	case errors.Is(err, usecase.ErrSyncDeviceRevoked):
		return "revoked"
	case errors.Is(err, synctrust.ErrKeysRotated):
		return "signIn"
	}
	return "integrity"
}

func deviceDTO(d usecase.DeviceInfo) SyncDeviceDTO {
	return SyncDeviceDTO{ID: hex.EncodeToString(d.Cert.Device[:]), Name: d.Cert.Name, Platform: d.Cert.Platform,
		CreatedUnixMilli: int64(min(d.Cert.CreatedUnixMilli, 1<<62)), Revoked: d.Revoked, Current: d.Current}
}

func heldDTO(h usecase.HeldChange) SyncHeldDTO {
	return SyncHeldDTO{Record: hex.EncodeToString(h.Record[:]), Type: string(h.Type), Key: h.Key, Reason: h.Reason,
		Pending: h.Pending, AtUnixMilli: h.AtUnixMilli}
}

// secretConflict reports a conflict over a field whose value is a secret.
func secretConflict(c domain.SyncConflict) bool {
	switch synctrust.RecordType(c.RecordType) {
	case synctrust.TypePassword, synctrust.TypePluginSecret:
		return true
	case synctrust.TypeIdentity:
		return len(c.Path) > 0 && c.Path[0] == "blob"
	}
	return false
}

func conflictDTO(c domain.SyncConflict) SyncConflictDTO {
	dto := SyncConflictDTO{ID: c.ID, RecordType: c.RecordType, ItemKey: c.ItemKey, Path: c.Path, Secret: secretConflict(c),
		AtUnixMilli: c.AtUnixMilli}
	if !dto.Secret {
		dto.Local, dto.Remote = string(c.Local), string(c.Remote)
	}
	return dto
}

// parseSyncID reads a 16-byte id the frontend got from this API.
func parseSyncID(s string) (vaultsync.ID, bool) {
	raw, err := hex.DecodeString(s)
	var id vaultsync.ID
	if err != nil || len(raw) != len(id) {
		return id, false
	}
	copy(id[:], raw)
	return id, !id.IsZero()
}
