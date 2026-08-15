package wails

import (
	"time"

	"xquakshell/internal/domain"
)

// PeerTrustDTO is one recorded remote identity, as the management screen sees it.
//
// There is no material field, and there will not be one. The screen exists to let a user recognise
// an entry and delete it; the bytes themselves answer neither question, and a DTO is the point
// where they would start travelling into a renderer, a log line and a devtools panel.
type PeerTrustDTO struct {
	Scope   string `json:"scope"`
	Subject string `json:"subject"`
	// Fingerprint is computed here from the stored material, by the same function the trust
	// dialog is rendered from. Two ways to fingerprint one value is how a screen ends up
	// disagreeing with the prompt the user actually answered.
	Fingerprint string `json:"fingerprint"`
	AddedAt     string `json:"addedAt"`
}

// PeerTrustToDTO maps stored entries for the UI.
func PeerTrustToDTO(entries []domain.PeerTrustEntry) []PeerTrustDTO {
	out := make([]PeerTrustDTO, 0, len(entries))
	for _, e := range entries {
		dto := PeerTrustDTO{
			Scope:       e.Scope,
			Subject:     e.Subject,
			Fingerprint: domain.PeerFingerprint(e.Material),
		}
		if !e.AddedAt.IsZero() {
			dto.AddedAt = e.AddedAt.UTC().Format(time.RFC3339)
		}
		out = append(out, dto)
	}
	return out
}
