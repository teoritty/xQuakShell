// A plugin that does nothing but remember which language the host told it about.
//
// It exists to exercise both halves of the i18n capability from a real process: the locale that
// rides the initialize handshake, and the i18n.localeChanged notification that follows every
// change. `locale.report` hands the answer back so a test can assert on what actually arrived
// rather than on what the host believes it sent.
package main

import (
	"encoding/json"
	"log"
	"sync"

	"xquakshell/test/fixtures/pluginhost"
)

func main() {
	host := pluginhost.NewHost()

	var mu sync.Mutex
	// Two separate fields, not one: a test that cannot tell "the handshake carried it" from "the
	// notification carried it" would pass with either half of the feature missing.
	initial := ""
	current := ""
	changes := 0

	host.Register("initialize", func(params json.RawMessage) (any, error) {
		var req struct {
			Locale string `json:"locale"`
		}
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		mu.Lock()
		initial, current = req.Locale, req.Locale
		mu.Unlock()
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

	host.RegisterNotification("i18n.localeChanged", func(params json.RawMessage) {
		var req struct {
			Locale string `json:"locale"`
		}
		if err := json.Unmarshal(params, &req); err != nil {
			return
		}
		mu.Lock()
		current = req.Locale
		changes++
		mu.Unlock()
	})

	host.Register("locale.report", func(_ json.RawMessage) (any, error) {
		mu.Lock()
		defer mu.Unlock()
		return map[string]any{"initial": initial, "current": current, "changes": changes}, nil
	})

	if err := host.Run(); err != nil {
		log.Fatal(err)
	}
}
