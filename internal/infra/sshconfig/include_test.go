package sshconfig

import (
	"path/filepath"
	"strings"
	"testing"

	"xquakshell/internal/domain"
)

func TestParseFollowsRelativeInclude(t *testing.T) {
	fakeHome(t)
	dir := t.TempDir()
	writeFile(t, dir, "extra.conf", "Host included\n  HostName included.example.com\n")
	path := writeFile(t, dir, "config", "Include extra.conf\nHost local\n")

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := hostByAlias(t, result, "included").HostName; got != "included.example.com" {
		t.Errorf("HostName = %q, want the included value", got)
	}
	if len(result.Hosts) != 2 {
		t.Errorf("got %d hosts, want 2 (included + local)", len(result.Hosts))
	}
}

func TestParseResolvesIncludeAgainstSSHDirectory(t *testing.T) {
	home := fakeHome(t)
	writeFile(t, filepath.Join(home, ".ssh"), "work.conf", "Host work\n  HostName work.example.com\n")
	// The root config lives elsewhere, so the only place `work.conf` can be
	// found is the OpenSSH-conventional ~/.ssh directory.
	path := writeFile(t, t.TempDir(), "config", "Include work.conf\n")

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := hostByAlias(t, result, "work").HostName; got != "work.example.com" {
		t.Errorf("HostName = %q, want the ~/.ssh include to be found", got)
	}
}

func TestParseExpandsIncludeGlob(t *testing.T) {
	fakeHome(t)
	dir := t.TempDir()
	writeFile(t, dir, "conf.d/a.conf", "Host alpha\n")
	writeFile(t, dir, "conf.d/b.conf", "Host beta\n")
	path := writeFile(t, dir, "config", "Include conf.d/*.conf\n")

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if got := strings.Join(aliases(result), ","); got != "alpha,beta" {
		t.Errorf("aliases = %q, want alpha,beta in sorted file order", got)
	}
}

func TestParseExpandsTildeInclude(t *testing.T) {
	home := fakeHome(t)
	writeFile(t, filepath.Join(home, "configs"), "extra.conf", "Host tilde\n")
	path := writeFile(t, t.TempDir(), "config", "Include ~/configs/extra.conf\n")

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(result.Hosts) != 1 || result.Hosts[0].Alias != "tilde" {
		t.Errorf("hosts = %v, want the ~-relative include to resolve", aliases(result))
	}
}

func TestParseReportsMissingInclude(t *testing.T) {
	fakeHome(t)
	path := writeFile(t, t.TempDir(), "config", "Include absent.conf\nHost web\n")

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(result.Hosts) != 1 {
		t.Errorf("a missing include must not abort the parse; got %d hosts", len(result.Hosts))
	}
	if !hasNotice(result, domain.SSHConfigNoticeIncludeUnreadable, "absent.conf") {
		t.Errorf("missing include must be reported; notices = %+v", result.Notices)
	}
}

func TestParseSurvivesIncludeCycle(t *testing.T) {
	fakeHome(t)
	dir := t.TempDir()
	writeFile(t, dir, "a.conf", "Host from-a\nInclude b.conf\n")
	writeFile(t, dir, "b.conf", "Host from-b\nInclude a.conf\n")
	path := writeFile(t, dir, "config", "Include a.conf\n")

	done := make(chan domain.SSHConfigParseResult, 1)
	go func() {
		result, err := Parse(path)
		if err != nil {
			t.Errorf("Parse: %v", err)
		}
		done <- result
	}()

	select {
	case result := <-done:
		if got := strings.Join(aliases(result), ","); got != "from-a,from-b" {
			t.Errorf("aliases = %q, want from-a,from-b each exactly once", got)
		}
	case <-timeoutAfterSeconds(10):
		t.Fatal("Parse did not terminate on a cyclic include")
	}
}

func TestParseStopsAtIncludeDepthLimit(t *testing.T) {
	fakeHome(t)
	dir := t.TempDir()
	// Each file includes the next, one level deeper than the limit allows.
	for i := 0; i < maxIncludeDepth+3; i++ {
		content := "Host level" + itoa(i) + "\nInclude level" + itoa(i+1) + ".conf\n"
		writeFile(t, dir, "level"+itoa(i)+".conf", content)
	}
	writeFile(t, dir, "level"+itoa(maxIncludeDepth+3)+".conf", "Host deepest\n")
	path := writeFile(t, dir, "config", "Include level0.conf\n")

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	for _, h := range result.Hosts {
		if h.Alias == "deepest" {
			t.Error("the include depth limit did not stop the descent")
		}
	}
	if !hasNotice(result, domain.SSHConfigNoticeLimitReached, "") {
		t.Errorf("hitting the depth limit must be reported; notices = %+v", result.Notices)
	}
}

func TestParseSkipsOversizeInclude(t *testing.T) {
	fakeHome(t)
	dir := t.TempDir()
	writeFile(t, dir, "big.conf", strings.Repeat("# padding\n", (maxFileSize/10)+1))
	path := writeFile(t, dir, "config", "Include big.conf\nHost web\n")

	result, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}

	if len(result.Hosts) != 1 || result.Hosts[0].Alias != "web" {
		t.Errorf("an oversize include must be skipped, not fatal; hosts = %v", aliases(result))
	}
	if !hasNotice(result, domain.SSHConfigNoticeIncludeUnreadable, "big.conf") {
		t.Errorf("oversize include must be reported; notices = %+v", result.Notices)
	}
}
