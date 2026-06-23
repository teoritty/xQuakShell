package main

import (
	"encoding/base64"
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
			welcome := fmt.Sprintf("Demo Telnet reference plugin connected to %s:%d\r\n> ", req.Host, req.Port)
			_, _ = host.CallCore("session.writeTerminal", pluginsdk.SessionTerminalParams{
				SessionID:    req.SessionID,
				OutputBase64: base64.StdEncoding.EncodeToString([]byte(welcome)),
			})
			_, _ = host.CallCore("session.updateState", pluginsdk.SessionUpdateParams{
				SessionID: req.SessionID,
				State:     "ready",
			})
		}()
		return map[string]bool{"accepted": true}, nil
	})

	host.RegisterNotification("session.writeInput", func(params json.RawMessage) {
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
		echo := append(data, []byte("\r\n> ")...)
		_, _ = host.CallCore("session.writeTerminal", pluginsdk.SessionTerminalParams{
			SessionID:    req.SessionID,
			OutputBase64: base64.StdEncoding.EncodeToString(echo),
		})
	})

	if err := host.Run(); err != nil {
		log.Fatal(err)
	}
}
