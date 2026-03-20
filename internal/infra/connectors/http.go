package connectors

import (
	"context"
	"net/url"
	"strings"

	"ssh-client/internal/domain"
)

// HTTPConnector marks the session ready; the UI loads the URL in an embedded WebView (iframe).
// We intentionally do not open the system browser so the app stays self-contained.
type HTTPConnector struct{}

// NewHTTPConnector creates an HTTP connector.
func NewHTTPConnector() *HTTPConnector {
	return &HTTPConnector{}
}

// Protocol implements domain.SessionConnector.
func (c *HTTPConnector) Protocol() string {
	return domain.ProtocolHTTP
}

// Connect implements domain.SessionConnector.
func (c *HTTPConnector) Connect(_ context.Context, conn *domain.Connection, _ domain.ConnectorDeps, hooks domain.ConnectorHooks) error {
	if conn.HTTPConfig == nil || strings.TrimSpace(conn.HTTPConfig.URL) == "" {
		hooks.UpdateState(domain.SessionError, "http url not configured")
		return nil
	}

	raw := strings.TrimSpace(conn.HTTPConfig.URL)
	if _, err := url.ParseRequestURI(raw); err != nil {
		// Allow bare host without scheme if user omitted it
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		if _, err2 := url.ParseRequestURI(raw); err2 != nil {
			hooks.UpdateState(domain.SessionError, "invalid http url")
			return nil
		}
	}

	hooks.UpdateState(domain.SessionReady, "")
	return nil
}
