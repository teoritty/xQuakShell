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

func cmdAdmin(ctx context.Context, args []string, out io.Writer) error {
	if len(args) < 2 {
		return errors.New("usage: xqsyncd admin invite create | account list|freeze|unfreeze|delete [username] --dir DIR")
	}
	fs := newFlags("admin")
	dir := fs.String("dir", "", "server data directory")
	ttl := fs.Duration("ttl", 24*time.Hour, "invite lifetime")
	if err := fs.Parse(args[2:]); err != nil {
		return err
	}
	k, err := readKeyFile(*dir)
	if err != nil {
		return err
	}
	svc, db, err := openService(ctx, *dir, k)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	switch args[0] + " " + args[1] {
	case "invite create":
		code, err := svc.CreateInvite(ctx, uint64(ttl.Milliseconds())) // #nosec G115 -- a flag duration, positive by construction
		if err == nil {
			_, err = fmt.Fprintln(out, code)
		}
		return err
	case "account list":
		accounts, err := svc.ListAccounts(ctx)
		for _, a := range accounts {
			fmt.Fprintf(out, "%s\t%s\tfrozen=%v\n", a.Username, a.ID, a.Frozen)
		}
		return err
	case "account freeze", "account unfreeze":
		return svc.SetFrozen(ctx, fs.Arg(0), args[1] == "freeze")
	case "account delete":
		return svc.DeleteAccount(ctx, fs.Arg(0))
	}
	return fmt.Errorf("xqsyncd admin: unknown command %q", args[0]+" "+args[1])
}
