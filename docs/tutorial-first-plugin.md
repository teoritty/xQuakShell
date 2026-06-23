# Tutorial: Your First Plugin

This walkthrough builds a minimal plugin from scratch and verifies it with `PingPlugin`.

## 1. Initialize

```bash
make plugin-cli
xqs-plugin init -id com.example.hello -name "Hello Plugin"
cd plugins/com.example.hello   # or your chosen output dir
```

## 2. Implement

Edit `main.go` to use the SDK:

```go
package main

import (
    "encoding/json"
    "log"

    "github.com/xquakshell/pluginsdk"
)

func main() {
    host := pluginsdk.NewHost()
    host.Register("initialize", func(json.RawMessage) (any, error) {
        return map[string]bool{"ok": true}, nil
    })
    host.Register("ping", func(json.RawMessage) (any, error) {
        return map[string]string{"pong": "ok"}, nil
    })
    host.Register("shutdown", func(json.RawMessage) (any, error) {
        return map[string]bool{"ok": true}, nil
    })
    log.Fatal(host.Run())
}
```

Ensure `plugin.json` lists `"engine": { "type": "go-binary", "entry": "hello.exe" }`.

## 3. Build and validate

```bash
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -trimpath -o hello.exe .
xqs-plugin validate
```

## 4. Install in xQuakShell

1. Run xQuakShell (dev build).
2. Open **Settings → Plugins**.
3. **Install folder…** and select your plugin directory.
4. Confirm permissions if the manifest requests vault or network access.

## 5. Start and ping

Plugins do not auto-start for ping. Use **Start** (dev) or trigger an `activationEvent`.

From the UI or Wails binding:

- `StartPlugin("com.example.hello")` — starts the process
- `PingPlugin("com.example.hello")` — expects `{ "pong": "ok" }`

## Next steps

- Add a command in `contributions.commands` and `activationEvents: ["onCommand:hello.ping"]`
- Add a WebView panel under `contributions.views` with external JS only (see `plugins/example-echo/ui/`)
- Read [plugin-api.md](./plugin-api.md) and [security-model.md](./security-model.md)

Reference implementations:

- `plugins/example-echo` — commands, views, status bar
- `plugins/demo-terminal` — session connector
