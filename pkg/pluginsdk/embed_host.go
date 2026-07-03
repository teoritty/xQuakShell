package pluginsdk

import (
	"context"
	"encoding/json"
	"sync"
)

// EmbedViewportParams is delivered via session.embedViewport notification.
type EmbedViewportParams struct {
	SessionID        string  `json:"sessionId"`
	WidthPx          int     `json:"widthPx"`
	HeightPx         int     `json:"heightPx"`
	DevicePixelRatio float64 `json:"devicePixelRatio"`
	Active           bool    `json:"active"`
}

// EmbedActivityParams is delivered via session.embedActivity notification.
type EmbedActivityParams struct {
	SessionID string `json:"sessionId"`
	Active    bool   `json:"active"`
}

// TunnelDataParams is delivered via session.tunnelData notification.
type TunnelDataParams struct {
	SessionID  string `json:"sessionId"`
	TunnelID   string `json:"tunnelId"`
	DataBase64 string `json:"dataBase64"`
}

// EmbedHostHandlers receives host-initiated embed notifications.
type EmbedHostHandlers struct {
	OnViewport func(EmbedViewportParams)
	OnActivity func(EmbedActivityParams)
	OnTunnelData func(TunnelDataParams)
	OnTunnelBackpressure func(sessionID string)
	OnTunnelResume func(sessionID string)
}

// RegisterEmbedHostHandlers wires embed host notifications on the plugin Host.
func RegisterEmbedHostHandlers(host *Host, h *EmbedHostHandlers) {
	if host == nil || h == nil {
		return
	}
	host.RegisterNotification("session.embedViewport", func(params json.RawMessage) {
		if h.OnViewport == nil {
			return
		}
		var p EmbedViewportParams
		if json.Unmarshal(params, &p) == nil {
			h.OnViewport(p)
		}
	})
	host.RegisterNotification("session.embedActivity", func(params json.RawMessage) {
		if h.OnActivity == nil {
			return
		}
		var p EmbedActivityParams
		if json.Unmarshal(params, &p) == nil {
			h.OnActivity(p)
		}
	})
	host.RegisterNotification("session.tunnelData", func(params json.RawMessage) {
		if h.OnTunnelData == nil {
			return
		}
		var p TunnelDataParams
		if json.Unmarshal(params, &p) == nil {
			h.OnTunnelData(p)
		}
	})
	host.RegisterNotification("session.tunnelBackpressure", func(params json.RawMessage) {
		if h.OnTunnelBackpressure == nil {
			return
		}
		var p struct {
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(params, &p) == nil {
			h.OnTunnelBackpressure(p.SessionID)
		}
	})
	host.RegisterNotification("session.tunnelResume", func(params json.RawMessage) {
		if h.OnTunnelResume == nil {
			return
		}
		var p struct {
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(params, &p) == nil {
			h.OnTunnelResume(p.SessionID)
		}
	})
}

// RunBidirectionalRelay copies bytes between a TCP handle and embed tunnel RPC.
func RunBidirectionalRelay(ctx context.Context, netClient *NetClient, embed *EmbedClient, handleID, sessionID, tunnelID string, pause <-chan struct{}) error {
	if netClient == nil || embed == nil {
		return nil
	}
	if err := embed.OpenTunnel(sessionID, tunnelID); err != nil {
		return err
	}

	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-ctx.Done():
				errCh <- ctx.Err()
				return
			case <-pause:
				continue
			default:
			}
			res, eof, err := netClient.Read(handleID, 32*1024)
			if err != nil {
				errCh <- err
				return
			}
			if err := embed.SendFrame(sessionID, tunnelID, res, eof); err != nil {
				errCh <- err
				return
			}
			if eof {
				return
			}
		}
	}()

	wg.Wait()
	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}
