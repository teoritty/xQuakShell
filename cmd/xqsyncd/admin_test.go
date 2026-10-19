package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func runOut(t *testing.T, args ...string) string {
	t.Helper()
	var out bytes.Buffer
	if err := run(context.Background(), args, &out); err != nil {
		t.Fatalf("xqsyncd %s: %v", strings.Join(args, " "), err)
	}
	return out.String()
}

func TestAdminInviteListAndRevoke(t *testing.T) {
	dir := initDir(t)
	code := strings.TrimSpace(runOut(t, "admin", "invite", "create", "--dir", dir))
	list := runOut(t, "admin", "invite", "list", "--dir", dir)
	if strings.Contains(list, code) {
		t.Fatal("the invite list shows a usable code")
	}
	fields := strings.Fields(list)
	if len(fields) < 2 || fields[len(fields)-1] != "unused" {
		t.Fatalf("list = %q; want one line: id, expiry, unused", list)
	}
	id := fields[0]
	runOut(t, "admin", "invite", "revoke", id, "--dir", dir)
	if got := runOut(t, "admin", "invite", "list", "--dir", dir); strings.TrimSpace(got) != "" {
		t.Fatalf("list after revoke = %q", got)
	}
	if err := run(context.Background(), []string{"admin", "invite", "revoke", id, "--dir", dir}, &bytes.Buffer{}); err == nil {
		t.Fatal("revoking a revoked invite succeeded")
	}
}

func TestAdminStats(t *testing.T) {
	dir := initDir(t)
	out := runOut(t, "admin", "stats", "--dir", dir)
	for _, want := range []string{"accounts\t0", "vaults\t0", "commits\t0", "bytes\t0"} {
		if !strings.Contains(out, want) {
			t.Errorf("stats %q; want a line %q", out, want)
		}
	}
}

// A backup is a consistent copy of the database alone - opened with the key file it is the server
// as it was - and backups older than --max-age are deleted (spec 10.3 R1: a backup outliving the
// trash window keeps what a purge removed).
func TestAdminBackupCopiesAndPrunes(t *testing.T) {
	dir := initDir(t)
	runOut(t, "admin", "invite", "create", "--dir", dir)
	to := filepath.Join(t.TempDir(), "backups")
	if err := os.MkdirAll(to, 0o700); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(to, "xqsyncd-20200101T000000Z.db")
	other := filepath.Join(to, "notes.txt")
	for _, p := range []string{old, other} {
		if err := os.WriteFile(p, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	past := time.Now().Add(-60 * 24 * time.Hour)
	if err := os.Chtimes(old, past, past); err != nil {
		t.Fatal(err)
	}
	out := runOut(t, "admin", "backup", "--dir", dir, "--to", to, "--max-age", "720h")
	matches, _ := filepath.Glob(filepath.Join(to, "xqsyncd-*.db"))
	if len(matches) != 1 || matches[0] == old {
		t.Fatalf("backups = %v (output %q); one new copy, the 60-day-old one deleted", matches, out)
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("a file that is not a backup was deleted")
	}
	if _, err := os.Stat(filepath.Join(to, keyFileName)); err == nil {
		t.Fatal("the backup carries the key file; the two must be kept apart (spec 9.4)")
	}

	restored := filepath.Join(t.TempDir(), "restored")
	if err := os.MkdirAll(restored, 0o700); err != nil {
		t.Fatal(err)
	}
	copyFile(t, matches[0], filepath.Join(restored, dbFileName))
	copyFile(t, filepath.Join(dir, keyFileName), filepath.Join(restored, keyFileName))
	if list := runOut(t, "admin", "invite", "list", "--dir", restored); !strings.Contains(list, "unused") {
		t.Fatalf("the restored copy lists %q; the invite made before the backup is in it", list)
	}
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	b, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, b, 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestServeQuotaFlags(t *testing.T) {
	cfg, addr, err := parseServe([]string{"--dir", "d", "--personal-quota-mib", "64", "--team-quota-mib", "2048", "--max-records", "500"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.personalQuota != 64<<20 || cfg.teamQuota != 2048<<20 || cfg.maxRecords != 500 || addr != ":8443" {
		t.Fatalf("cfg = %+v, addr %q", cfg, addr)
	}
	cfg, _, _ = parseServe([]string{"--dir", "d"})
	if cfg.personalQuota != 0 || cfg.teamQuota != 0 || cfg.maxRecords != 0 {
		t.Fatal("without the flags the use case's defaults (256 MiB, 1 GiB) apply")
	}
}

// The usage puts the account name before --dir; the flag package alone would stop at the name and
// never see --dir, failing on a key file it then looked for in the working directory.
func TestAdminFlagsAfterTheAccountName(t *testing.T) {
	dir := initDir(t)
	err := run(context.Background(), []string{"admin", "account", "freeze", "nobody", "--dir", dir}, &bytes.Buffer{})
	if err == nil || strings.Contains(err.Error(), "key file") {
		t.Fatalf("freeze NAME --dir D: %v; want the unknown-account refusal from the right directory", err)
	}
}

// A container has no shell to run init before serve, so serve --init does it. Run on every start,
// it must create the directory once and then leave the keys alone: a new key file on a restart would
// show every user a different fingerprint and lock every account out.
func TestServeInitFirstInitialisesOnceAndKeepsTheKeys(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	cfg, _, err := parseServe([]string{"--dir", dir, "--init", "--host", "sync.example.org", "--host", "10.0.0.5"})
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.initFirst {
		t.Fatal("--init was not recorded")
	}
	var first, second bytes.Buffer
	if err := cmdInit(initArgs(cfg), &first); err != nil {
		t.Fatal(err)
	}
	k1, err := readKeyFile(dir)
	if err != nil {
		t.Fatalf("serve --init on an empty directory left no key file: %v", err)
	}
	if !strings.Contains(first.String(), "sync.example.org") {
		t.Fatalf("output %q; the connect link names the first --host", first.String())
	}
	if err := cmdInit(initArgs(cfg), &second); err != nil {
		t.Fatal(err)
	}
	k2, _ := readKeyFile(dir)
	if identityFingerprint(k1) != identityFingerprint(k2) {
		t.Fatal("a second start replaced the identity key; every client would see a different server")
	}
	if !strings.Contains(second.String(), identityFingerprint(k1)) {
		t.Fatalf("output %q; a restart still prints the fingerprint for the operator", second.String())
	}
	if cfg, _, _ := parseServe([]string{"--dir", dir}); cfg.initFirst {
		t.Fatal("serve without --init must not initialise anything")
	}
}

func TestServeRateFlags(t *testing.T) {
	cfg, _, err := parseServe([]string{"--dir", "d", "--rate-per-second", "50", "--burst", "500"})
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ratePerSecond != 50 || cfg.burst != 500 {
		t.Fatalf("rate %v, burst %d; an operator behind a shared NAT raises both", cfg.ratePerSecond, cfg.burst)
	}
	if cfg, _, _ = parseServe([]string{"--dir", "d"}); cfg.ratePerSecond != 0 || cfg.burst != 0 {
		t.Fatal("without the flags the transport's defaults apply")
	}
}
