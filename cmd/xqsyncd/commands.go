package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func newFlags(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	return fs
}

func orDefault(value, dir, name string) string {
	if value != "" {
		return value
	}
	return filepath.Join(dir, name)
}

func listen(ctx context.Context, addr string) (net.Listener, error) {
	var lc net.ListenConfig
	return lc.Listen(ctx, "tcp", addr)
}

type hostList []string

func (h *hostList) String() string     { return strings.Join(*h, ",") }
func (h *hostList) Set(v string) error { *h = append(*h, v); return nil }

func cmdInit(args []string, out io.Writer) error {
	fs := newFlags("init")
	dir := fs.String("dir", "", "server data directory")
	var hosts hostList
	fs.Var(&hosts, "host", "host name or address for the self-signed certificate (repeatable)")
	// A service manager runs init before every start; on an initialised directory it must change
	// nothing, and still print the fingerprint into the service log for the operator to read.
	keep := fs.Bool("keep-existing", false, "succeed without changes when the directory is initialised")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *dir == "" {
		return errors.New("xqsyncd init: --dir is required")
	}
	if len(hosts) == 0 {
		hosts = hostList{"localhost"}
	}
	if err := os.MkdirAll(*dir, 0o700); err != nil {
		return err
	}
	if existing, err := readKeyFile(*dir); err == nil && *keep {
		_, err = fmt.Fprintf(out, "already initialised %s\nidentity fingerprint: %s\n", *dir, identityFingerprint(existing))
		return err
	}
	k, err := newKeyFile(defaultRand)
	if err != nil {
		return err
	}
	if err := writeKeyFile(*dir, k); err != nil {
		return err
	}
	if err := writeSelfSignedCert(*dir, hosts, time.Now()); err != nil {
		return err
	}
	_, err = fmt.Fprintf(out, "initialised %s\nidentity fingerprint: %s\n", *dir, identityFingerprint(k))
	return err
}
