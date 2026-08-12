package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"xquakshell/test/fixtures/pluginhost"
)

// announcePID writes this process's pid into the temp directory the host hands every plugin. It is
// how a test learns which OS process to look for: a host that loses track of a plugin mid-start
// has, by definition, no pid to report, and that is exactly the case worth testing.
//
// It used to write one directory above the installed bundle, which is outside everything a
// confined plugin may touch — so on Linux the fixture died at its first statement and every test
// that uses it failed at the handshake. Writing where a real plugin writes is what makes this
// fixture a plugin rather than a process that happens to speak the protocol.
//
// A failure here is fatal rather than ignored. Tests read "no pid file" as "this process was killed
// before it ran", and a swallowed write error would forge that answer for a process that is very
// much alive. Exiting instead makes the two cases stay different: a fixture that cannot name itself
// says so, loudly, and does not linger to be mistaken for a corpse.
func announcePID() {
	path := filepath.Join(os.TempDir(), "slow-start.pid")
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		log.Fatalf("slow-start fixture cannot announce pid %d at %s: %v", os.Getpid(), path, err)
	}
}

func main() {
	announcePID()
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
