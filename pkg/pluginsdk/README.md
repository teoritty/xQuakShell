# github.com/xquakshell/pluginsdk

Go SDK for building xQuakShell out-of-process plugins.

## Requirements

- Go 1.25+
- xQuakShell core API 1.0.0+

## Install

```bash
go get github.com/xquakshell/pluginsdk@v0.1.0
```

For local development against the xQuakShell repository:

```go
replace github.com/xquakshell/pluginsdk => ../pkg/pluginsdk
```

## Quick start

```go
host := pluginsdk.NewHost()
host.Register("initialize", func(params json.RawMessage) (any, error) {
    return map[string]bool{"ok": true}, nil
})
client := pluginsdk.NewClient(host)
_ = client // call core via client.Ping(), client.CallCore(...)
host.Run()
```

See `plugins/example-echo` in the xQuakShell repository for a complete reference plugin.

## Compatibility

Match `minCoreVersion` in your `plugin.json` to the host you target. The SDK NDJSON frame limit is 256 KiB (same as core IPC).
