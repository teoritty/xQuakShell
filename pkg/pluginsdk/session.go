package pluginsdk

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strconv"
)

// SessionHandler wires common session RPC handlers for a plugin protocol.
type SessionHandler struct {
	OnConnect    func(params SessionConnectParams) error
	OnInput      func(sessionID string, data []byte) error
	OnResize     func(sessionID string, cols, rows int) error
	OnDisconnect func(sessionID string) error
}

// GetString returns a string field value from session.connect params.
func (p *SessionConnectParams) GetString(fieldID string) string {
	if p == nil || p.Fields == nil {
		return ""
	}
	return p.Fields[fieldID]
}

// GetInt returns an integer field value from session.connect params.
func (p *SessionConnectParams) GetInt(fieldID string) (int, error) {
	v, ok := p.Fields[fieldID]
	if !ok {
		return 0, fmt.Errorf("field %q not found", fieldID)
	}
	return strconv.Atoi(v)
}

// GetFloat returns a floating-point field value from session.connect params.
func (p *SessionConnectParams) GetFloat(fieldID string) (float64, error) {
	v, ok := p.Fields[fieldID]
	if !ok {
		return 0, fmt.Errorf("field %q not found", fieldID)
	}
	return strconv.ParseFloat(v, 64)
}

// GetBool returns a boolean field value from session.connect params.
func (p *SessionConnectParams) GetBool(fieldID string) bool {
	return p.Fields[fieldID] == "true"
}

// RegisterSessionHandler registers session lifecycle RPC handlers on the plugin host.
func RegisterSessionHandler(host *Host, handler *SessionHandler) {
	if host == nil || handler == nil {
		return
	}

	host.Register("session.connect", func(params json.RawMessage) (any, error) {
		var req SessionConnectParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if handler.OnConnect == nil {
			return map[string]bool{"accepted": true}, nil
		}
		if err := handler.OnConnect(req); err != nil {
			return nil, err
		}
		return map[string]bool{"accepted": true}, nil
	})

	host.RegisterNotification("session.writeInput", func(params json.RawMessage) {
		if handler.OnInput == nil {
			return
		}
		var req struct {
			SessionID  string `json:"sessionId"`
			DataBase64 string `json:"dataBase64"`
		}
		if err := json.Unmarshal(params, &req); err != nil {
			return
		}
		data, err := base64.StdEncoding.DecodeString(req.DataBase64)
		if err != nil {
			return
		}
		_ = handler.OnInput(req.SessionID, data)
	})

	host.RegisterNotification("session.disconnect", func(params json.RawMessage) {
		if handler.OnDisconnect == nil {
			return
		}
		var req struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(params, &req); err != nil {
			return
		}
		_ = handler.OnDisconnect(req.SessionID)
	})
}
