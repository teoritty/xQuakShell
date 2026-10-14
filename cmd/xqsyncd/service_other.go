//go:build !windows

package main

import (
	"errors"
	"io"
)

var errServiceUnsupported = errors.New("xqsyncd service: on this system install the systemd unit from packaging/linux; see docs/sync-server.md")

func installService([]string, io.Writer) error   { return errServiceUnsupported }
func uninstallService([]string, io.Writer) error { return errServiceUnsupported }

// runIfService reports that this process was not started by a service manager it must answer to:
// systemd runs serve as an ordinary process.
func runIfService([]string) (bool, error) { return false, nil }
