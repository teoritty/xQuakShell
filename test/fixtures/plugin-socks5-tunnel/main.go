package main

import (
	"encoding/json"
	"log"

	"ssh-client/test/fixtures/pluginhost"
)

func main() {
	host := pluginhost.NewHost()

	host.Register("initialize", func(_ json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})
	host.Register("activate", func(_ json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})
	host.Register("shutdown", func(_ json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})
	host.RegisterNotification("tunnel.localAccept", func(params json.RawMessage) {
		var req struct {
			RuleID      string `json:"ruleId"`
			LocalConnID string `json:"localConnId"`
		}
		if err := json.Unmarshal(params, &req); err != nil {
			return
		}
		_, _ = host.CallCore("tunnel.dial", map[string]any{
			"targetHost": "example.com",
			"targetPort": 80,
		})
		_, _ = host.CallCore("tunnel.bind", map[string]string{
			"localConnId": req.LocalConnID,
			"tunnelId":    "stub-tunnel",
		})
	})

	if err := host.Run(); err != nil {
		log.Fatal(err)
	}
}
