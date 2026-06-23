package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/xquakshell/pluginsdk"
)

type sessionState struct {
	id   string
	host string
	port int
}

func main() {
	host := pluginsdk.NewHost()
	sessions := make(map[string]*sessionState)
	var mu sync.Mutex

	host.Register("initialize", func(params json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})

	host.Register("ping", func(_ json.RawMessage) (any, error) {
		return map[string]string{"pong": "ok"}, nil
	})

	host.OnShutdown(func(_ json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})

	host.Register("activate", func(params json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})

	host.OnDeactivate(func(_ json.RawMessage) {
		log.Printf("demo-terminal deactivated")
	})

	host.Register("session.connect", func(params json.RawMessage) (any, error) {
		var req pluginsdk.SessionConnectParams
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}

		mu.Lock()
		sessions[req.SessionID] = &sessionState{id: req.SessionID, host: req.Host, port: req.Port}
		mu.Unlock()

		go func() {
			welcome := fmt.Sprintf("Demo Terminal connected to %s:%d\r\nType to echo.\r\n> ", req.Host, req.Port)
			if err := writeTerminal(host, req.SessionID, []byte(welcome)); err != nil {
				log.Printf("writeTerminal failed: %v", err)
				return
			}
			_, err := host.CallCore("session.updateState", pluginsdk.SessionUpdateParams{
				SessionID: req.SessionID,
				State:     "ready",
			})
			if err != nil {
				log.Printf("updateState failed: %v", err)
			}
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
		_ = writeTerminal(host, req.SessionID, echo)
	})

	host.RegisterNotification("session.disconnect", func(params json.RawMessage) {
		var req struct {
			SessionID string `json:"sessionId"`
		}
		if err := json.Unmarshal(params, &req); err != nil {
			return
		}
		mu.Lock()
		delete(sessions, req.SessionID)
		mu.Unlock()
	})

	if err := host.Run(); err != nil {
		log.Fatal(err)
	}
}

func writeTerminal(host *pluginsdk.Host, sessionID string, data []byte) error {
	_, err := host.CallCore("session.writeTerminal", pluginsdk.SessionTerminalParams{
		SessionID:    sessionID,
		OutputBase64: base64.StdEncoding.EncodeToString(data),
	})
	return err
}
