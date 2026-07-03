package pluginsdk

import (
	"encoding/base64"
	"fmt"
)

// RegisterEmbedParams is sent via session.registerEmbed RPC.
type RegisterEmbedParams struct {
	SessionID string   `json:"sessionId"`
	UIEntry   string   `json:"uiEntry"`
	TunnelIDs []string `json:"tunnelIds,omitempty"`
}

// RegisterEmbedResult is returned by session.registerEmbed.
type RegisterEmbedResult struct {
	EmbedToken string `json:"embedToken"`
	UIUrl      string `json:"uiUrl"`
	TunnelUrl  string `json:"tunnelUrl"`
	ExpiresAt  string `json:"expiresAt"`
}

// EmbedClient wraps session embed RPC for plugins.
type EmbedClient struct {
	client *Client
}

// Embed returns the embed RPC client.
func (c *Client) Embed() *EmbedClient {
	if c == nil {
		return nil
	}
	return &EmbedClient{client: c}
}

// Register mints a session-scoped embed token via the core broker.
func (e *EmbedClient) Register(params RegisterEmbedParams) (RegisterEmbedResult, error) {
	var out RegisterEmbedResult
	if e == nil || e.client == nil {
		return out, fmt.Errorf("embed client unavailable")
	}
	if err := e.client.CallCore("session.registerEmbed", params, &out); err != nil {
		return out, err
	}
	return out, nil
}

// OpenTunnel notifies the host that a tunnel endpoint is ready.
func (e *EmbedClient) OpenTunnel(sessionID, tunnelID string) error {
	if e == nil || e.client == nil {
		return fmt.Errorf("embed client unavailable")
	}
	return e.client.CallCore("session.tunnelOpen", map[string]string{
		"sessionId": sessionID,
		"tunnelId":  tunnelID,
	}, nil)
}

// SendFrame sends opaque bytes host → browser via the embed broker.
func (e *EmbedClient) SendFrame(sessionID, tunnelID string, data []byte, eof bool) error {
	if e == nil || e.client == nil {
		return fmt.Errorf("embed client unavailable")
	}
	return e.client.CallCore("session.tunnelFrame", map[string]any{
		"sessionId":  sessionID,
		"tunnelId":   tunnelID,
		"dataBase64": base64.StdEncoding.EncodeToString(data),
		"eof":        eof,
	}, nil)
}

// CloseTunnel closes a tunnel for the session.
func (e *EmbedClient) CloseTunnel(sessionID, tunnelID string) error {
	if e == nil || e.client == nil {
		return fmt.Errorf("embed client unavailable")
	}
	return e.client.CallCore("session.tunnelClose", map[string]string{
		"sessionId": sessionID,
		"tunnelId":  tunnelID,
	}, nil)
}
