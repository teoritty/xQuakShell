package sshconfig

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"xquakshell/internal/domain"
)

// writeFile creates a file under dir, creating parent directories as needed,
// and returns its path.
func writeFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", name, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
	return path
}

// fakeHome points os.UserHomeDir at a temporary directory so tests can exercise
// ~ expansion and the ~/.ssh include fallback without touching the real home.
func fakeHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return home
}

// hostByAlias finds a parsed host, failing the test when it is absent.
func hostByAlias(t *testing.T, result domain.SSHConfigParseResult, alias string) domain.SSHConfigHost {
	t.Helper()
	for _, h := range result.Hosts {
		if h.Alias == alias {
			return h
		}
	}
	t.Fatalf("host %q not found in %d parsed hosts", alias, len(result.Hosts))
	return domain.SSHConfigHost{}
}

func hasNotice(result domain.SSHConfigParseResult, kind domain.SSHConfigNoticeKind, target string) bool {
	for _, n := range result.Notices {
		if n.Kind == kind && (target == "" || n.Target == target) {
			return true
		}
	}
	return false
}

func aliases(result domain.SSHConfigParseResult) []string {
	out := make([]string, 0, len(result.Hosts))
	for _, h := range result.Hosts {
		out = append(out, h.Alias)
	}
	return out
}

// Defaults are conventionally written at the end of an ssh_config, because
// OpenSSH keeps the first value it obtains. This is the layout users actually
// have, and the one where "specific beats general" holds.
func TestParseResolvesHostsWithTrailingWildcardDefaults(t *testing.T) {
	fakeHome(t)
	dir := t.TempDir()
	path := writeFile(t, dir, "config", `
Host web
    HostName web.example.com
    User deploy

Host db
    HostName db.internal

Host *
    User defaultuser
    Port 2222
`)

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := aliases(result); strings.Join(got, ",") != "web,db" {
		t.Fatalf("aliases = %v, want [web db] — wildcard blocks must not become hosts", got)
	}

	web := hostByAlias(t, result, "web")
	if web.HostName != "web.example.com" || web.Port != 2222 {
		t.Errorf("web = %+v, want hostname web.example.com port 2222", web)
	}
	if web.User != "deploy" {
		t.Errorf("web user = %q, want deploy — the specific block was read first", web.User)
	}

	db := hostByAlias(t, result, "db")
	if db.User != "defaultuser" {
		t.Errorf("db user = %q, want defaultuser inherited from Host *", db.User)
	}
}

// The mirror image: a leading `Host *` wins over a later specific block. This
// looks backwards but is exactly what ssh(1) does, and importing a host with
// different settings than ssh would use is worse than importing surprising
// ones — the connection has to behave the way the config says.
func TestParseLeadingWildcardWinsLikeOpenSSH(t *testing.T) {
	fakeHome(t)
	path := writeFile(t, t.TempDir(), "config", `
Host *
    User defaultuser

Host web
    HostName web.example.com
    User deploy
`)

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	web := hostByAlias(t, result, "web")
	if web.User != "defaultuser" {
		t.Errorf("web user = %q, want defaultuser — the first value obtained wins", web.User)
	}
	if web.HostName != "web.example.com" {
		t.Errorf("web hostname = %q, want web.example.com", web.HostName)
	}
}

func TestParseFirstValueWins(t *testing.T) {
	fakeHome(t)
	path := writeFile(t, t.TempDir(), "config", `
Host web
    HostName first.example.com
    HostName second.example.com

Host web
    HostName third.example.com
`)

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := hostByAlias(t, result, "web").HostName; got != "first.example.com" {
		t.Errorf("HostName = %q, want first.example.com (OpenSSH keeps the first value obtained)", got)
	}
	if len(result.Hosts) != 1 {
		t.Errorf("a repeated alias must yield one host, got %d", len(result.Hosts))
	}
}

func TestParseAppliesHostNameTokenAndAliasFallback(t *testing.T) {
	fakeHome(t)
	path := writeFile(t, t.TempDir(), "config", `
Host *.internal
    HostName %h.example.com

Host box.internal
Host plain
`)

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := hostByAlias(t, result, "box.internal").HostName; got != "box.internal.example.com" {
		t.Errorf("HostName = %q, want %%h expanded to the alias", got)
	}
	if got := hostByAlias(t, result, "plain").HostName; got != "plain" {
		t.Errorf("HostName = %q, want the alias as fallback", got)
	}
}

func TestParseHonoursNegatedPatterns(t *testing.T) {
	fakeHome(t)
	path := writeFile(t, t.TempDir(), "config", `
Host web-* !web-staging
    User deploy

Host web-1
Host web-staging
`)

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := hostByAlias(t, result, "web-1").User; got != "deploy" {
		t.Errorf("web-1 user = %q, want deploy", got)
	}
	if got := hostByAlias(t, result, "web-staging").User; got != "" {
		t.Errorf("web-staging user = %q, want empty — the negated pattern excludes it", got)
	}
}

func TestParseRejectsInvalidPort(t *testing.T) {
	fakeHome(t)
	path := writeFile(t, t.TempDir(), "config", "Host web\n  Port 99999\n")

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := hostByAlias(t, result, "web").Port; got != 0 {
		t.Errorf("Port = %d, want 0 so the caller applies the SSH default", got)
	}
}

func TestParseReportsMatchBlocks(t *testing.T) {
	fakeHome(t)
	path := writeFile(t, t.TempDir(), "config", `
Host web
    HostName web.example.com

Match host web exec "true"
    User conditional
`)

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := hostByAlias(t, result, "web").User; got != "" {
		t.Errorf("user = %q, want empty — Match conditions cannot be evaluated statically", got)
	}
	if !hasNotice(result, domain.SSHConfigNoticeMatchBlockSkipped, "") {
		t.Error("a skipped Match block must be reported to the user")
	}
}

func TestParseReportsProxyCommand(t *testing.T) {
	fakeHome(t)
	path := writeFile(t, t.TempDir(), "config", "Host web\n  ProxyCommand nc %h %p\n")

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if !hasNotice(result, domain.SSHConfigNoticeProxyCommandUnsupported, "web") {
		t.Error("ProxyCommand has no jump-chain equivalent and must be reported")
	}
}

func TestParseMissingFile(t *testing.T) {
	fakeHome(t)

	_, err := Parse(filepath.Join(t.TempDir(), "absent"))

	if !errors.Is(err, domain.ErrSSHConfigNotFound) {
		t.Fatalf("err = %v, want ErrSSHConfigNotFound", err)
	}
}

func TestParseRefusesDirectory(t *testing.T) {
	fakeHome(t)
	dir := t.TempDir()

	_, err := Parse(dir)

	if !errors.Is(err, domain.ErrSSHConfigUnreadable) {
		t.Fatalf("err = %v, want ErrSSHConfigUnreadable", err)
	}
}

func TestParseRefusesOversizeFile(t *testing.T) {
	fakeHome(t)
	path := writeFile(t, t.TempDir(), "config", strings.Repeat("# padding\n", (maxFileSize/10)+1))

	_, err := Parse(path)

	if !errors.Is(err, domain.ErrSSHConfigTooLarge) {
		t.Fatalf("err = %v, want ErrSSHConfigTooLarge", err)
	}
}

func TestParseCapsHostCount(t *testing.T) {
	fakeHome(t)
	var sb strings.Builder
	for i := 0; i < maxHosts+10; i++ {
		sb.WriteString("Host h")
		sb.WriteString(strings.Repeat("x", i%3))
		sb.WriteString(itoa(i))
		sb.WriteString("\n")
	}
	path := writeFile(t, t.TempDir(), "config", sb.String())

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(result.Hosts) != maxHosts {
		t.Errorf("got %d hosts, want the cap of %d", len(result.Hosts), maxHosts)
	}
	if !hasNotice(result, domain.SSHConfigNoticeLimitReached, "") {
		t.Error("hitting the host cap must be reported so the user knows the list is partial")
	}
}

// timeoutAfterSeconds is a small helper for tests that must prove termination
// rather than merely observe a result.
func timeoutAfterSeconds(n int) <-chan time.Time {
	return time.After(time.Duration(n) * time.Second)
}

// itoa avoids importing strconv purely for test fixture generation.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var digits []byte
	for n > 0 {
		digits = append([]byte{byte('0' + n%10)}, digits...)
		n /= 10
	}
	return string(digits)
}
