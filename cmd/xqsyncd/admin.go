package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"xquakshell/internal/syncserver/store"
	"xquakshell/internal/syncserver/usecase"
)

const adminUsage = "usage: xqsyncd admin invite create|list|revoke ID | account list|freeze|unfreeze|delete [username] | stats | backup --to DIR [--max-age 720h] --dir DIR"

// adminCmd is one administrative command over the opened service and database.
type adminCmd func(ctx context.Context, a adminRun) error

type adminRun struct {
	svc *usecase.Service
	db  *store.DB
	arg string
	out io.Writer
	ttl time.Duration
	to  string
	age time.Duration
}

var adminCommands = map[string]adminCmd{
	"invite create":    adminInviteCreate,
	"invite list":      adminInviteList,
	"invite revoke":    func(ctx context.Context, a adminRun) error { return a.svc.RevokeInvite(ctx, a.arg) },
	"account list":     adminAccountList,
	"account freeze":   func(ctx context.Context, a adminRun) error { return a.svc.SetFrozen(ctx, a.arg, true) },
	"account unfreeze": func(ctx context.Context, a adminRun) error { return a.svc.SetFrozen(ctx, a.arg, false) },
	"account delete":   func(ctx context.Context, a adminRun) error { return a.svc.DeleteAccount(ctx, a.arg) },
	"stats":            adminStats,
	"backup":           adminBackup,
}

// cmdAdmin works on the database directly. There is no administrative network API (spec 9.3).
func cmdAdmin(ctx context.Context, args []string, out io.Writer) error {
	name, rest := adminCommandName(args)
	command, ok := adminCommands[name]
	if !ok {
		return errors.New(adminUsage)
	}
	fs := newFlags("admin")
	dir := fs.String("dir", "", "server data directory")
	ttl := fs.Duration("ttl", 24*time.Hour, "invite lifetime")
	to := fs.String("to", "", "backup directory")
	age := fs.Duration("max-age", 30*24*time.Hour, "delete backups older than this (0 keeps them all)")
	positional, err := parseInterspersed(fs, rest)
	if err != nil {
		return err
	}
	var arg string
	if len(positional) > 0 {
		arg = positional[0]
	}
	k, err := readKeyFile(*dir)
	if err != nil {
		return err
	}
	svc, db, err := openService(ctx, serveConfig{dir: *dir}, k)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	return command(ctx, adminRun{svc: svc, db: db, arg: arg, out: out, ttl: *ttl, to: *to, age: *age})
}

// parseInterspersed parses flags wherever they appear: the flag package stops at the first
// positional argument, which silently dropped --dir from "account freeze NAME --dir D".
func parseInterspersed(fs *flag.FlagSet, args []string) ([]string, error) {
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		if fs.NArg() == 0 {
			return positional, nil
		}
		positional = append(positional, fs.Arg(0))
		args = fs.Args()[1:]
	}
}

// adminCommandName takes "stats" and "backup" alone, every other command as two words.
func adminCommandName(args []string) (string, []string) {
	if len(args) >= 1 && (args[0] == "stats" || args[0] == "backup") {
		return args[0], args[1:]
	}
	if len(args) >= 2 {
		return args[0] + " " + args[1], args[2:]
	}
	return "", nil
}

func adminInviteCreate(ctx context.Context, a adminRun) error {
	code, err := a.svc.CreateInvite(ctx, uint64(a.ttl.Milliseconds())) // #nosec G115 -- a flag duration, positive by construction
	if err == nil {
		_, err = fmt.Fprintln(a.out, code)
	}
	return err
}

func adminInviteList(ctx context.Context, a adminRun) error {
	list, err := a.svc.ListInvites(ctx)
	for _, inv := range list {
		state := "unused"
		if inv.Used {
			state = "used"
		}
		expires := time.UnixMilli(int64(inv.Expires)).UTC().Format(time.RFC3339) // #nosec G115 -- milliseconds since 1970 fit an int64
		fmt.Fprintf(a.out, "%s\t%s\t%s\n", inv.ID, expires, state)
	}
	return err
}

func adminAccountList(ctx context.Context, a adminRun) error {
	accounts, err := a.svc.ListAccounts(ctx)
	for _, acc := range accounts {
		fmt.Fprintf(a.out, "%s\t%s\tfrozen=%v\n", acc.Username, acc.ID, acc.Frozen)
	}
	return err
}

func adminStats(ctx context.Context, a adminRun) error {
	st, err := a.svc.Stats(ctx)
	if err != nil {
		return err
	}
	_, err = fmt.Fprintf(a.out, "accounts\t%d\nfrozen\t%d\nvaults\t%d\ncommits\t%d\nbytes\t%d\n",
		st.Accounts, st.Frozen, st.Vaults, st.Commits, st.Bytes)
	return err
}

// adminBackup writes a copy of the database - never the key file, which is backed up once and kept
// apart (spec 9.4) - and deletes backups older than --max-age. A backup that outlives the trash
// window still holds what a purge removed (spec 10.3 R1), so the default is the default window.
func adminBackup(ctx context.Context, a adminRun) error {
	if a.to == "" {
		return errors.New("xqsyncd admin backup: --to is required")
	}
	if err := os.MkdirAll(a.to, 0o700); err != nil {
		return err
	}
	name := filepath.Join(a.to, "xqsyncd-"+time.Now().UTC().Format("20060102T150405Z")+".db")
	if err := a.db.BackupTo(ctx, name); err != nil {
		return err
	}
	fmt.Fprintf(a.out, "wrote %s\n", name)
	if a.age <= 0 {
		return nil
	}
	return pruneBackups(a.to, name, time.Now().Add(-a.age), a.out)
}

func pruneBackups(dir, keep string, before time.Time, out io.Writer) error {
	old, err := filepath.Glob(filepath.Join(dir, "xqsyncd-*.db"))
	if err != nil {
		return err
	}
	for _, p := range old {
		info, err := os.Stat(p)
		if err != nil || p == keep || !strings.HasSuffix(p, ".db") || !info.ModTime().Before(before) {
			continue
		}
		if err := os.Remove(p); err != nil {
			return err
		}
		fmt.Fprintf(out, "removed %s\n", p)
	}
	return nil
}
