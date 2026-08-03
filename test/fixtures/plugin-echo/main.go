package main

import (
	"encoding/json"
	"log"

	"xquakshell/test/fixtures/pluginhost"
)

func main() {
	host := pluginhost.NewHost()

	host.Register("initialize", func(_ json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})
	host.Register("activate", func(_ json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})
	host.Register("ping", func(_ json.RawMessage) (any, error) {
		return map[string]string{"pong": "ok"}, nil
	})
	host.Register("shutdown", func(_ json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})
	host.Register("command.execute", func(params json.RawMessage) (any, error) {
		var req struct {
			CommandID string `json:"commandId"`
		}
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		if req.CommandID == "echo.ping" {
			return map[string]string{"message": "Echo ping OK"}, nil
		}
		return nil, nil
	})
	host.RegisterNotification("view.postMessage", func(params json.RawMessage) {
		var req struct {
			PanelID string          `json:"panelId"`
			Message json.RawMessage `json:"message"`
		}
		if err := json.Unmarshal(params, &req); err != nil {
			return
		}
		_, _ = host.CallCore("view.postMessage", map[string]any{
			"panelId": req.PanelID,
			"message": map[string]string{"echo": "pong from plugin"},
		})
	})

	if err := host.Run(); err != nil {
		log.Fatal(err)
	}
}
