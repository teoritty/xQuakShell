package wails

import (
	"time"

	domainplugin "xquakshell/internal/domain/plugin"
)

// PluginSourceDTO is one row of the plugin screen's source list.
//
// Timestamps cross as RFC3339 strings rather than as time.Time, matching GitHubRepositoryDTO:
// Wails marshals a zero time.Time to a date in year 1, which the UI would have to special-case in
// every place it formats one.
type PluginSourceDTO struct {
	ID                string `json:"id"`
	Kind              string `json:"kind"`
	DisplayName       string `json:"displayName"`
	Trusted           bool   `json:"trusted"`
	Removable         bool   `json:"removable"`
	Available         bool   `json:"available"`
	UnavailableReason string `json:"unavailableReason,omitempty"`
	AddedAt           string `json:"addedAt,omitempty"`
	LastFetchedAt     string `json:"lastFetchedAt,omitempty"`
}

func pluginSourceToDTO(source domainplugin.PluginSource) PluginSourceDTO {
	dto := PluginSourceDTO{
		ID:                source.ID,
		Kind:              string(source.Kind),
		DisplayName:       source.DisplayName,
		Trusted:           source.Trusted,
		Removable:         source.Removable,
		Available:         source.Available,
		UnavailableReason: source.UnavailableReason,
	}
	if !source.AddedAt.IsZero() {
		dto.AddedAt = source.AddedAt.Format(time.RFC3339)
	}
	if source.LastFetchedAt != nil {
		dto.LastFetchedAt = source.LastFetchedAt.Format(time.RFC3339)
	}
	return dto
}
