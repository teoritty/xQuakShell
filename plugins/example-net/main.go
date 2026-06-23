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
		return map[string]bool{"ok": true}, nil
	})

	host.Register("activate", func(params json.RawMessage) (any, error) {
		handleID, err := client.Net().Dial("127.0.0.1", 9)
		if err != nil {
			return nil, err
		}
		defer func() { _ = client.Net().Close(handleID) }()
		_, _, _ = client.Net().Read(handleID, 1024)
		return map[string]string{"handleId": handleID}, nil
	})

	host.OnShutdown(func(_ json.RawMessage) (any, error) {
		return map[string]bool{"ok": true}, nil
	})

	if err := host.Run(); err != nil {
		log.Fatal(err)
	}
}
