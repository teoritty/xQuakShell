package main

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"xquakshell/test/fixtures/escapeprobe"
)

// dialTimeout bounds every network vector. A blocked socket is often a silence rather than a
// refusal — an AppContainer drops the packets — so "it never answered" has to become an error
// quickly enough that a test suite still finishes.
const dialTimeout = 2 * time.Second

// portableVectors are the attacks that mean the same thing on every platform this runs on.
func portableVectors() map[string]func(escapeprobe.Target) error {
	return map[string]func(escapeprobe.Target) error{
		escapeprobe.VectorReadForeign:      readTarget,
		escapeprobe.VectorSymlinkEscape:    readTarget,
		escapeprobe.VectorDotDotTraversal:  readTarget,
		escapeprobe.VectorReadOtherPlugin:  readTarget,
		escapeprobe.VectorReadVault:        readTarget,
		escapeprobe.VectorWriteForeign:     writeIntoTarget,
		escapeprobe.VectorWriteInstallTree: writeIntoTarget,
		escapeprobe.VectorListPluginsRoot:  listTarget,
		escapeprobe.VectorDialTCP:          dialTCP,
		escapeprobe.VectorDialUDP:          echoOverUDP,
		escapeprobe.VectorWriteThenExec:    writeThenExec,
	}
}

func readTarget(target escapeprobe.Target) error {
	_, err := os.ReadFile(target.Path)
	return err
}

func writeIntoTarget(target escapeprobe.Target) error {
	return os.WriteFile(filepath.Join(target.Path, "escaped"), []byte("x"), 0o600)
}

func listTarget(target escapeprobe.Target) error {
	_, err := os.ReadDir(target.Path)
	return err
}

func dialTCP(target escapeprobe.Target) error {
	conn, err := net.DialTimeout("tcp", target.Addr, dialTimeout)
	if err != nil {
		return err
	}
	return conn.Close()
}

// echoOverUDP insists on a datagram making the round trip. Creating a UDP socket and connecting it
// does no I/O at all, so a probe that stopped there would report success against a boundary that
// blocks every packet.
func echoOverUDP(target escapeprobe.Target) error {
	conn, err := net.DialTimeout("udp", target.Addr, dialTimeout)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()

	if err := conn.SetDeadline(time.Now().Add(dialTimeout)); err != nil {
		return err
	}
	if _, err := conn.Write([]byte("ping")); err != nil {
		return err
	}
	reply := make([]byte, 8)
	_, err = conn.Read(reply)
	return err
}

// writeThenExec copies this binary into the directory the plugin may write and runs the copy.
//
// The copy is the honest way to ask the question. Executing something already in the install tree
// proves nothing — that tree is granted execute on purpose — and a shell script would test whether
// the interpreter is reachable instead of whether the data directory carries execute.
func writeThenExec(target escapeprobe.Target) error {
	self, err := os.Executable()
	if err != nil {
		return err
	}
	body, err := os.ReadFile(self)
	if err != nil {
		return err
	}

	name := "copied-probe"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	copied := filepath.Join(target.Path, name)
	if err := os.WriteFile(copied, body, 0o700); err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), dialTimeout)
	defer cancel()
	// An empty request: the copy is being run to find out whether it CAN run, and a copy that
	// probed anything would make this vector's outcome depend on another vector's.
	return exec.CommandContext(ctx, copied, `{"vectors":{}}`).Run()
}
