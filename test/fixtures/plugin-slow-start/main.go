package main

import (
	"encoding/json"
	"log"
	"time"

	"ssh-client/test/fixtures/pluginhost"
)

func main() {
	host := pluginhost.NewHost()

	host.Register("initialize", func(_ json.RawMessage) (any, error) {
		time.Sleep(2 * time.Second)
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

	if err := host.Run(); err != nil {
		log.Fatal(err)
	}
}
