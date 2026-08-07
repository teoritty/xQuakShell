package wails

import (
	"fmt"

	wailsrt "github.com/wailsapp/wails/v2/pkg/runtime"

	"xquakshell/internal/domain"
	"xquakshell/internal/usecase"
)

// --- Settings ---

// GetSettings returns the current application settings.
func (a *AppAPI) GetSettings() (AppSettingsDTO, error) {
	s, err := a.settingsSvc.GetSettings()
	if err != nil {
		return AppSettingsDTO{}, err
	}
	return AppSettingsToDTO(s), nil
}

// SaveSettings persists application settings to the vault and applies them.
func (a *AppAPI) SaveSettings(dto AppSettingsDTO) error {
	settings := DTOToAppSettings(dto)
	if err := a.settingsSvc.SaveSettings(a.reqCtx(), settings); err != nil {
		return err
	}
	if a.auditSvc != nil {
		_ = a.auditSvc.EnforceRetention(a.reqCtx())
	}
	if a.pingMgr != nil {
		a.pingMgr.Stop()
		a.pingMgr.Start(func(results []usecase.PingResult) {
			if a.ctx != nil {
				dtos := make([]PingResultDTO, 0, len(results))
				for _, r := range results {
					dtos = append(dtos, PingResultDTO{ConnectionID: r.ConnectionID, Reachable: r.Reachable, LatencyMs: r.LatencyMs})
				}
				wailsrt.EventsEmit(a.ctx, EventPingUpdated, dtos)
			}
		})
	}
	if a.logLevel != nil {
		a.logLevel.SetLevel(settings.Debug.LogLevel)
	}
	a.SyncDebugLogWindow(settings.Debug.LogWindowEnabled)
	return nil
}

// --- Ping ---

// GetPingResults returns the current ping results for all connections.
func (a *AppAPI) GetPingResults() []PingResultDTO {
	if a.pingMgr == nil {
		return nil
	}
	raw := a.pingMgr.GetResults()
	result := make([]PingResultDTO, 0, len(raw))
	for _, r := range raw {
		result = append(result, PingResultDTO{
			ConnectionID: r.ConnectionID,
			Reachable:    r.Reachable,
			LatencyMs:    r.LatencyMs,
		})
	}
	return result
}

// PingConnection pings a single connection immediately.
func (a *AppAPI) PingConnection(connID string) {
	if a.pingMgr != nil {
		a.pingMgr.PingByConnectionID(a.reqCtx(), connID)
	}
}

// --- Audit Log ---

// SearchAuditLog performs full-text search on audit entries.
func (a *AppAPI) SearchAuditLog(query, sessionID, connectionID, category string, limit, offset int) ([]AuditEntryDTO, error) {
	if a.auditSvc == nil {
		return nil, fmt.Errorf("audit log not available")
	}
	filter := domain.AuditSearchFilter{
		Category:     category,
		SessionID:    sessionID,
		ConnectionID: connectionID,
		Limit:        limit,
		Offset:       offset,
	}
	entries, err := a.auditSvc.Search(a.reqCtx(), query, filter)
	if err != nil {
		return nil, err
	}
	return AuditEntriesToDTO(entries), nil
}

// DeleteAuditEntry removes a single audit log entry by ID.
func (a *AppAPI) DeleteAuditEntry(id int64) error {
	if a.auditSvc == nil {
		return fmt.Errorf("audit log not available")
	}
	return a.auditSvc.DeleteByID(a.reqCtx(), id)
}

// ClearAuditLog removes audit log entries of the given category. An empty
// category clears all entries.
func (a *AppAPI) ClearAuditLog(category string) error {
	if a.auditSvc == nil {
		return fmt.Errorf("audit log not available")
	}
	return a.auditSvc.ClearAll(a.reqCtx(), category)
}

// AuditSessionStateDTO exposes session-only audit options to the UI.
type AuditSessionStateDTO struct {
	LogSecretsEnabled bool `json:"logSecretsEnabled"`
}

// GetAuditSessionState returns live session audit flags (not persisted in vault).
func (a *AppAPI) GetAuditSessionState() AuditSessionStateDTO {
	if a.auditSvc == nil {
		return AuditSessionStateDTO{}
	}
	return AuditSessionStateDTO{LogSecretsEnabled: a.auditSvc.SessionLogSecretsEnabled()}
}

// EnableAuditSecretLogging enables plaintext secret logging for this session only.
func (a *AppAPI) EnableAuditSecretLogging(confirmed bool) error {
	if !confirmed {
		return fmt.Errorf("confirmation required")
	}
	if a.auditSvc == nil {
		return fmt.Errorf("audit log not available")
	}
	a.auditSvc.EnableSessionSecretLogging()
	return nil
}

// DisableAuditSecretLogging turns off session secret logging.
func (a *AppAPI) DisableAuditSecretLogging() {
	if a.auditSvc != nil {
		a.auditSvc.DisableSessionSecretLogging()
	}
}
