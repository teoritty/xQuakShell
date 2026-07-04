package main

import (
	"encoding/base64"
	"encoding/json"
	"log"
	"sync"

	"ssh-client/test/fixtures/pluginhost"
)

func main() {
	host := pluginhost.NewHost()
	var sessionMu sync.Mutex
	var sessionID string

	host.Register("initialize", func(_ json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})
	host.Register("activate", func(_ json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})
	host.Register("shutdown", func(_ json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})
	host.Register("session.connect", func(params json.RawMessage) (any, error) {
		var req struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		sessionMu.Lock()
		sessionID = req.SessionID
		sessionMu.Unlock()

		go func() {
			sid := req.SessionID
			_, _ = host.CallCore("session.writeTerminal", map[string]string{
				"sessionId":    sid,
				"outputBase64": base64.StdEncoding.EncodeToString([]byte("Demo Terminal\r\n")),
			})
			_, _ = host.CallCore("session.updateState", map[string]string{
				"sessionId": sid,
				"state":     "ready",
			})
		}()
		return map[string]bool{"accepted": true}, nil
	})
	host.RegisterNotification("session.writeInput", func(params json.RawMessage) {
		var req struct {
			DataBase64 string `json:"dataBase64"`
		}
		if err := json.Unmarshal(params, &req); err != nil {
			return
		}
		sessionMu.Lock()
		sid := sessionID
		sessionMu.Unlock()
		if sid == "" {
			return
		}
		_, _ = host.CallCore("session.writeTerminal", map[string]string{
			"sessionId":    sid,
			"outputBase64": req.DataBase64,
		})
	})

	if err := host.Run(); err != nil {
		log.Fatal(err)
	}
}
