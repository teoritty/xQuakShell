# Example Events Plugin

Reference plugin for xQuakShell event bus:

- Subscribes to `core.session.opened`
- Publishes to `plugin.com.xquakshell.example-events.ready` on activate

Build:

```bash
go build -ldflags="-s -w" -trimpath -o example-events.exe .
```

Copy the binary next to `plugin.json` under `data/plugins/com.xquakshell.example-events/`.
