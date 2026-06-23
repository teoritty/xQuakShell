package main

import (
	"encoding/json"
	"log"

	"github.com/xquakshell/pluginsdk"
)

func main() {
	host := pluginsdk.NewHost()
	client := pluginsdk.NewClient(host)

	host.Register("initialize", func(params json.RawMessage) (any, error) {
		_ = client.CallCore("events.subscribe", map[string]string{
			"channel": "core.session.opened",
		}, nil)
		return map[string]bool{"ok": true}, nil
	})

	host.Register("ping", func(_ json.RawMessage) (any, error) {
		return map[string]string{"plugin": "example-events", "status": "ok"}, nil
	})

	host.Register("activate", func(_ json.RawMessage) (any, error) {
		payload, _ := json.Marshal(map[string]string{"hello": "events"})
		_ = client.CallCore("events.publish", map[string]any{
			"channel": "plugin.com.xquakshell.example-events.ready",
			"payload": json.RawMessage(payload),
		}, nil)
		return map[string]bool{"ok": true}, nil
	})

	host.OnShutdown(func(_ json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})

	log.Println("example-events plugin running")
	_ = host.Run()
}
