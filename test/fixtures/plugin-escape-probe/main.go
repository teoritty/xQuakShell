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
	"log"
	"os"
	"time"

	"xquakshell/test/fixtures/escapeprobe"
	"xquakshell/test/fixtures/pluginhost"
)

// flagPark keeps the process alive doing nothing, so a test can offer it as something to attach to.
// The ptrace vector needs a victim that is outside the sandbox and expendable: attaching to the
// test process itself and failing to detach would leave the whole suite stopped.
const flagPark = "--park"

// parkDuration outlives any single test but not a forgotten process. The test kills it; this is the
// backstop for the run that crashes before it can.
const parkDuration = 2 * time.Minute

func probe(req escapeprobe.Request) escapeprobe.Report {
	vectors := allVectors()
	report := escapeprobe.Report{Vectors: make(map[string]escapeprobe.Outcome, len(req.Vectors))}
	for name, target := range req.Vectors {
		run, known := vectors[name]
		if !known {
			// Not "not attempted": an unknown vector means the two binaries disagree about the
			// contract, and that has to read as a broken probe rather than as a denied attempt.
			report.Vectors[name] = escapeprobe.Outcome{Err: "this probe has no vector called " + name}
			continue
		}
		report.Vectors[name] = attempt(func() error { return run(target) })
	}
	return report
}

// attempt runs one vector and reports whether it got through.
func attempt(do func() error) escapeprobe.Outcome {
	if err := do(); err != nil {
		return escapeprobe.Outcome{Attempted: true, Err: err.Error()}
	}
	return escapeprobe.Outcome{Attempted: true, Succeeded: true}
}

// main speaks the plugin protocol, except when the test runs it directly with a request on argv.
// That direct mode is the control arm: the same probes, the same binary, no sandbox. Without it a
// probe with a typo in a path would report "denied" forever and the confined test would pass on
// nothing.
func main() {
	if len(os.Args) > 1 {
		if os.Args[1] == flagPark {
			time.Sleep(parkDuration)
			return
		}
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
		var req escapeprobe.Request
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
	var req escapeprobe.Request
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

// allVectors is every attack this binary knows, portable ones plus whatever this platform adds.
func allVectors() map[string]func(escapeprobe.Target) error {
	vectors := portableVectors()
	for name, run := range platformVectors() {
		if _, clash := vectors[name]; clash {
			// Two implementations of one name would make the report depend on map iteration order.
			log.Fatalf("escape probe defines %s twice", name)
		}
		vectors[name] = run
	}
	return vectors
}
