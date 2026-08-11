// Command plugin-escape-probe tries to step outside a plugin sandbox and reports what happened.
//
// It is the only honest way to test a sandbox. Asserting that the ruleset was applied tests the
// caller; asserting that a process behind it cannot read a file it was not granted tests the
// kernel, and that is the claim the UI makes to the user.
//
// The paths it attacks arrive from the test, never from here. A fixture with ~/.ssh baked into it
// is a fixture that reads a developer's real keys on every `make test` run, and one bad merge away
// from being a fixture that reports them.
package main

import (
	"encoding/json"
	"errors"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"xquakshell/test/fixtures/pluginhost"
)

// probeRequest is what the test asks for: which paths to attack, and where to dial.
type probeRequest struct {
	ReadPath string `json:"readPath"`
	WriteDir string `json:"writeDir"`
	DialAddr string `json:"dialAddr"`
}

// probeResult reports one attempt. Denied is the answer the test cares about; Err carries the
// detail, because "it failed" and "it failed for the reason the sandbox exists" are different
// results and a test that cannot tell them apart passes for the wrong reason.
type probeResult struct {
	Attempted bool   `json:"attempted"`
	Denied    bool   `json:"denied"`
	Err       string `json:"err,omitempty"`
}

type probeReport struct {
	Read  probeResult `json:"read"`
	Write probeResult `json:"write"`
	Dial  probeResult `json:"dial"`
}

func probe(req probeRequest) probeReport {
	return probeReport{
		Read: attempt(req.ReadPath, func() error { _, err := os.ReadFile(req.ReadPath); return err }),
		Write: attempt(req.WriteDir, func() error {
			return os.WriteFile(filepath.Join(req.WriteDir, "escaped"), []byte("x"), 0o600)
		}),
		Dial: attempt(req.DialAddr, func() error {
			conn, err := net.DialTimeout("tcp", req.DialAddr, 2*time.Second)
			if err == nil {
				_ = conn.Close()
			}
			return err
		}),
	}
}

// attempt runs one probe and classifies the outcome. A permission error is the sandbox working; any
// other error is reported as not-denied with the detail attached, so a test cannot mistake a typo
// in a path for a boundary that held.
func attempt(target string, do func() error) probeResult {
	if target == "" {
		return probeResult{}
	}
	err := do()
	switch {
	case err == nil:
		return probeResult{Attempted: true}
	case errors.Is(err, os.ErrPermission):
		return probeResult{Attempted: true, Denied: true, Err: err.Error()}
	default:
		return probeResult{Attempted: true, Err: err.Error()}
	}
}

// main speaks the plugin protocol, except when the test runs it directly with a request on argv.
// That direct mode is the control arm: the same probes, the same binary, no sandbox. Without it a
// probe with a typo in it would report "denied" forever and the confined test would pass on
// nothing.
func main() {
	if len(os.Args) > 1 {
		runDirect(os.Args[1])
		return
	}

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
	host.Register("probe.run", func(params json.RawMessage) (any, error) {
		var req probeRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, err
		}
		return probe(req), nil
	})

	if err := host.Run(); err != nil {
		log.Fatal(err)
	}
}

func runDirect(raw string) {
	var req probeRequest
	if err := json.Unmarshal([]byte(raw), &req); err != nil {
		log.Fatalf("escape probe cannot read its request: %v", err)
	}
	out, err := json.Marshal(probe(req))
	if err != nil {
		log.Fatalf("escape probe cannot report: %v", err)
	}
	if _, err := os.Stdout.Write(out); err != nil {
		log.Fatalf("escape probe cannot write its report: %v", err)
	}
}
