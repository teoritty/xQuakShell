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
	host.Register("shutdown", func(_ json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})
	host.Register("auth.prepare", func(raw json.RawMessage) (any, error) {
		var req struct {
			ConnectionID string            `json:"connectionId"`
			Fields       map[string]string `json:"fields"`
		}
		_ = json.Unmarshal(raw, &req)
		_ = req.ConnectionID
		_ = req.Fields
		return map[string]string{"publicKeyBlobBase64": ""}, nil
	})
	host.Register("auth.answerChallenge", func(_ json.RawMessage) (any, error) {
		return map[string][]string{"answers": {"123456"}}, nil
	})

	if err := host.Run(); err != nil {
		log.Fatal(err)
	}
}
