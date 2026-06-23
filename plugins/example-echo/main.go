package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/xquakshell/pluginsdk"
)

func main() {
	host := pluginsdk.NewHost()
	_ = pluginsdk.NewClient(host)

	host.Register("initialize", func(params json.RawMessage) (any, error) {
		var init pluginsdk.InitializeParams
		if len(params) > 0 {
			_ = json.Unmarshal(params, &init)
		}
		log.Printf("example-echo initialized pluginId=%s api=%s", init.PluginID, init.APIVersion)
		return map[string]bool{"ok": true}, nil
	})

	host.Register("activate", func(params json.RawMessage) (any, error) {
		var req struct {
			Reason string `json:"reason"`
		}
		_ = json.Unmarshal(params, &req)
		log.Printf("example-echo activated reason=%s", req.Reason)
		return map[string]bool{"ok": true}, nil
	})

	host.OnDeactivate(func(_ json.RawMessage) {
		log.Printf("example-echo deactivated")
	})

	host.OnShutdown(func(_ json.RawMessage) (any, error) {
		log.Printf("example-echo shutdown acknowledged")
		return map[string]bool{"ok": true}, nil
	})

	host.Register("command.execute", func(params json.RawMessage) (any, error) {
		var req struct {
			CommandID string `json:"commandId"`
		}
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		switch req.CommandID {
		case "echo.ping":
			return map[string]string{"message": "Echo ping OK"}, nil
		default:
			return nil, fmt.Errorf("unknown command %q", req.CommandID)
		}
	})

	host.RegisterNotification("view.postMessage", func(params json.RawMessage) {
		var req struct {
			PanelID string          `json:"panelId"`
			Message json.RawMessage `json:"message"`
		}
		if err := json.Unmarshal(params, &req); err != nil {
			return
		}
		var msg struct {
			Action string `json:"action"`
		}
		_ = json.Unmarshal(req.Message, &msg)
		reply := map[string]string{"echo": "pong from plugin", "action": msg.Action}
		_, _ = host.CallCore("view.postMessage", pluginsdk.ViewPostMessageParams{
			PanelID: req.PanelID,
			Message: reply,
		})
	})

	host.Register("ping", func(_ json.RawMessage) (any, error) {
		return map[string]string{"pong": "ok"}, nil
	})

	host.OnShutdown(func(_ json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})

	if err := host.Run(); err != nil {
		log.Fatal(err)
	}
}
