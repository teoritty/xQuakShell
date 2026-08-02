package wails

import (
	"time"

	"xquakshell/internal/domain"
)

type SessionDTO struct {
	SessionID      string           `json:"sessionId"`
	ConnectionID   string           `json:"connectionId"`
	ConnectionName string           `json:"connectionName"`
	Protocol       string           `json:"protocol,omitempty"`
	Surface        string           `json:"surface,omitempty"`
	State          string           `json:"state"`
	ErrorMessage   string           `json:"errorMessage"`
	Embed          *SessionEmbedDTO `json:"embed,omitempty"`
}

type SessionEmbedDTO struct {
	UIUrl     string   `json:"uiUrl"`
	TunnelUrl string   `json:"tunnelUrl"`
	Sandbox   []string `json:"sandbox"`
}

type SessionEmbedReadyPayload struct {
	SessionID string          `json:"sessionId"`
	Embed     SessionEmbedDTO `json:"embed"`
}

type RemoteNodeDTO struct {
	Path    string `json:"path"`
	Name    string `json:"name"`
	IsDir   bool   `json:"isDir"`
	Size    int64  `json:"size"`
	ModTime string `json:"modTime"`
	Mode    string `json:"mode,omitempty"`
	Owner   string `json:"owner,omitempty"`
	Group   string `json:"group,omitempty"`
}

type AuditEntryDTO struct {
	ID             int64  `json:"id"`
	Timestamp      string `json:"timestamp"`
	Category       string `json:"category"`
	SessionID      string `json:"sessionId"`
	ConnectionID   string `json:"connectionId"`
	ConnectionName string `json:"connectionName"`
	Host           string `json:"host"`
	Username       string `json:"username"`
	Input          string `json:"input"`
	Redacted       bool   `json:"redacted"`
}

type PingResultDTO struct {
	ConnectionID string `json:"connectionId"`
	Reachable    bool   `json:"reachable"`
	LatencyMs    int64  `json:"latencyMs"`
}

func SessionToDTO(s domain.ConnectionSession) SessionDTO {
	return SessionDTO{
		SessionID:      s.SessionID,
		ConnectionID:   s.ConnectionID,
		ConnectionName: s.ConnectionName,
		Protocol:       s.Protocol,
		Surface:        s.Surface,
		State:          string(s.State),
		ErrorMessage:   s.ErrorMessage,
	}
}

func SessionEmbedToDTO(desc domain.SessionEmbedDescriptor) SessionEmbedReadyPayload {
	return SessionEmbedReadyPayload{
		SessionID: desc.SessionID,
		Embed: SessionEmbedDTO{
			UIUrl:     desc.UIUrl,
			TunnelUrl: desc.TunnelUrl,
			Sandbox:   desc.Sandbox,
		},
	}
}

func RemoteNodeToDTO(n domain.RemoteNode) RemoteNodeDTO {
	return RemoteNodeDTO{
		Path:    n.Path,
		Name:    n.Name,
		IsDir:   n.IsDir,
		Size:    n.Size,
		ModTime: n.ModTime.Format("2006-01-02 15:04:05"),
		Mode:    n.Mode,
		Owner:   n.Owner,
		Group:   n.Group,
	}
}

func RemoteNodesToDTO(ns []domain.RemoteNode) []RemoteNodeDTO {
	result := make([]RemoteNodeDTO, len(ns))
	for i, n := range ns {
		result[i] = RemoteNodeToDTO(n)
	}
	return result
}

func AuditEntryToDTO(e domain.AuditEntry) AuditEntryDTO {
	return AuditEntryDTO{
		ID:             e.ID,
		Timestamp:      e.Timestamp.Format(time.RFC3339),
		Category:       e.Category,
		SessionID:      e.SessionID,
		ConnectionID:   e.ConnectionID,
		ConnectionName: e.ConnectionName,
		Host:           e.Host,
		Username:       e.Username,
		Input:          e.Input,
		Redacted:       e.Redacted,
	}
}

func AuditEntriesToDTO(entries []domain.AuditEntry) []AuditEntryDTO {
	result := make([]AuditEntryDTO, len(entries))
	for i, e := range entries {
		result[i] = AuditEntryToDTO(e)
	}
	return result
}
