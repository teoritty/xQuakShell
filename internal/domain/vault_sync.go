package domain

import (
	"encoding/json"
	"strconv"
	"strings"
)

// VaultSync is this device's sync state inside the encrypted vault (ADR-023). The field is additive
// and omitempty like PeerTrust, so the vault schema does not move: an older build that drops it loses
// only what is listed here and has to sign in to sync again.
type VaultSync struct {
	// Conflicts are credential fields two devices set to different values. The vault keeps the value
	// the server accepted first; the other one waits here, encrypted with the rest of the vault and
	// synced to every device, until a person picks one.
	Conflicts []SyncConflict `json:"conflicts,omitempty"`
}

// SyncConflict is one unresolved credential conflict.
type SyncConflict struct {
	// ID is ConflictID(RecordType, ItemKey, Path); a second conflict on the same field replaces the
	// first rather than piling up beside it.
	ID          string          `json:"id"`
	RecordType  string          `json:"recordType"`
	ItemKey     string          `json:"itemKey"`
	Path        []string        `json:"path"`
	Local       json.RawMessage `json:"local,omitempty"`
	Remote      json.RawMessage `json:"remote,omitempty"`
	AtUnixMilli int64           `json:"at"`
}

// ConflictID names the conflict on one field of one item. Each part is quoted, so no separator can
// be forged: item keys may contain any character, including the newline peer-trust keys use.
func ConflictID(recordType, itemKey string, path []string) string {
	parts := make([]string, 0, len(path)+2)
	for _, p := range append([]string{recordType, itemKey}, path...) {
		parts = append(parts, strconv.Quote(p))
	}
	return strings.Join(parts, ",")
}
