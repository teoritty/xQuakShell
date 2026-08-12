//go:build !linux

package main

import "xquakshell/test/fixtures/escapeprobe"

// platformVectors is empty away from Linux: /proc and ptrace have no counterpart here, and a
// fake implementation that always failed would report a boundary holding where there is no door.
// A vector this platform has no version of is left out of the request by the test instead.
func platformVectors() map[string]func(escapeprobe.Target) error {
	return nil
}
