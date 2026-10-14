// Command xqsyncd is the self-hosted xQuakShell sync server (ADR-023). It stores ciphertext, public
// keys and signed logs, and cannot open any vault it holds.
//
//	xqsyncd init --dir D [--host H ...]   create the key file and a self-signed TLS certificate
//	xqsyncd serve --dir D [--addr :8443] [--personal-quota-mib 256] [--team-quota-mib 1024]
//	xqsyncd fingerprint --dir D            print the identity fingerprint clients compare
//	xqsyncd admin invite create [--ttl 24h] | invite list | invite revoke ID --dir D
//	xqsyncd admin account list|freeze|unfreeze|delete [username] --dir D
//	xqsyncd admin stats --dir D
//	xqsyncd admin backup --to B [--max-age 720h] --dir D   (the database only; back server.key up apart)
//	xqsyncd service install|uninstall [--name N] [--dir D] [--addr A]   (Windows; systemd units ship in packaging/linux)
//
// Administration works on the database directly. There is no administrative network API: it would
// be attack surface on the one machine that holds every user's ciphertext, for no user benefit.
package main

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"xquakshell/internal/domain/vaultsync"
)

func main() {
	// Under the Windows service control manager the process answers to it instead of to signals.
	if handled, err := runIfService(os.Args[1:]); handled || err != nil {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	// The composition root owns the process lifetime: this is the one place a root context is right.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	err := run(ctx, os.Args[1:], os.Stdout)
	stop()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string, out io.Writer) error {
	if len(args) == 0 {
		return usageError()
	}
	switch args[0] {
	case "init":
		return cmdInit(args[1:], out)
	case "serve":
		return cmdServe(ctx, args[1:])
	case "fingerprint":
		return cmdFingerprint(args[1:], out)
	case "admin":
		return cmdAdmin(ctx, args[1:], out)
	case "service":
		return cmdService(ctx, args[1:], out)
	default:
		return usageError()
	}
}

func usageError() error {
	return fmt.Errorf("usage: xqsyncd init|serve|fingerprint|admin|service ... --dir DIR")
}

func cmdFingerprint(args []string, out io.Writer) error {
	fs := newFlags("fingerprint")
	dir := fs.String("dir", "", "server data directory")
	if err := fs.Parse(args); err != nil {
		return err
	}
	k, err := readKeyFile(*dir)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintln(out, identityFingerprint(k))
	return err
}

// identityFingerprint is what an operator reads out and a user compares on first connection.
func identityFingerprint(k *keyFile) string {
	return vaultsync.AccountFingerprint(ed25519.NewKeyFromSeed(k.IdentitySeed).Public().(ed25519.PublicKey))
}

func cmdServe(ctx context.Context, args []string) error {
	cfg, addr, err := parseServe(args)
	if err != nil {
		return err
	}
	ln, err := listen(ctx, addr)
	if err != nil {
		return err
	}
	return serve(ctx, cfg, ln)
}

func parseServe(args []string) (serveConfig, string, error) {
	fs := newFlags("serve")
	dir := fs.String("dir", "", "server data directory")
	addr := fs.String("addr", ":8443", "listen address")
	cert := fs.String("tls-cert", "", "TLS certificate (default DIR/tls.crt)")
	key := fs.String("tls-key", "", "TLS key (default DIR/tls.key)")
	personal := fs.Uint64("personal-quota-mib", 0, "ciphertext quota per personal vault in MiB (default 256)")
	team := fs.Uint64("team-quota-mib", 0, "ciphertext quota per team vault in MiB (default 1024)")
	if err := fs.Parse(args); err != nil {
		return serveConfig{}, "", err
	}
	return serveConfig{dir: *dir, certPath: orDefault(*cert, *dir, certFileName), keyPath: orDefault(*key, *dir, keyPEMName),
		log: slog.New(slog.NewTextHandler(logOutput, nil)), personalQuota: *personal << 20, teamQuota: *team << 20}, *addr, nil
}
