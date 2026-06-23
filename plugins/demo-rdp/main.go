package main

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/xquakshell/pluginsdk"
)

func main() {
	host := pluginsdk.NewHost()

	host.Register("initialize", func(params json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})
	host.Register("ping", func(_ json.RawMessage) (any, error) {
		return map[string]string{"pong": "ok"}, nil
	})
	host.OnShutdown(func(_ json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})
	host.Register("activate", func(_ json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})

	host.Register("session.connect", func(params json.RawMessage) (any, error) {
		var req pluginsdk.SessionConnectParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		go func() {
			msg := fmt.Sprintf(
				"RDP is handled by an external desktop client in this reference plugin. Target: %s:%d user=%s",
				req.Host, req.Port, req.Username,
			)
			_ = pluginsdk.LogInfo(host, "rdp connect requested", map[string]string{
				"host":     req.Host,
				"protocol": req.Protocol,
			})
			_, _ = host.CallCore("session.updateState", pluginsdk.SessionUpdateParams{
				SessionID: req.SessionID,
				State:     "error",
				Error:     msg,
			})
		}()
		return map[string]bool{"accepted": true}, nil
	})

	if err := host.Run(); err != nil {
		log.Fatal(err)
	}
}
