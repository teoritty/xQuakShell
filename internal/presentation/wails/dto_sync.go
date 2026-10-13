package wails

import (
	"encoding/hex"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/usecase"
)

// SyncStatusDTO is the sync indicator and the header of the settings page.
type SyncStatusDTO struct {
	Enabled      bool     `json:"enabled"`
	Running      bool     `json:"running"`
	State        string   `json:"state"`
	Message      string   `json:"message,omitempty"`
	Held         int      `json:"held"`
	Quarantined  int      `json:"quarantined"`
	NewDevices   []string `json:"newDevices,omitempty"`
	ServerURL    string   `json:"serverUrl,omitempty"`
	Username     string   `json:"username,omitempty"`
	Fingerprint  string   `json:"fingerprint,omitempty"`
	RekeyPending bool     `json:"rekeyPending,omitempty"`
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
	for _, d := range st.Report.Pull.NewDevices {
		dto.NewDevices = append(dto.NewDevices, d.Name)
	}
	return dto
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
