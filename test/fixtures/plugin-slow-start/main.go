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

// announcePID writes this process's pid one directory above the installed bundle — outside it, so
// the file cannot break the bundle's set-equality checksum check. It is how a test learns which OS
// process to look for: a host that loses track of a plugin mid-start has, by definition, no pid to
// report, and that is exactly the case worth testing.
//
// A failure here is fatal rather than ignored. Tests read "no pid file" as "this process was killed
// before it ran", and a swallowed write error would forge that answer for a process that is very
// much alive. Exiting instead makes the two cases stay different: a fixture that cannot name itself
// says so, loudly, and does not linger to be mistaken for a corpse.
func announcePID() {
	exe, err := os.Executable()
	if err != nil {
		log.Fatalf("slow-start fixture cannot locate itself: %v", err)
	}
	path := filepath.Join(filepath.Dir(filepath.Dir(exe)), "slow-start.pid")
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
