package release_test

import (
	"bufio"
	"strings"
	"testing"
)

const xqsyncdUnit = "xqsyncd.service"

// unitDirectives reads a systemd unit into key -> values (a key may repeat).
func unitDirectives(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	sc := bufio.NewScanner(strings.NewReader(readPackagingFile(t, xqsyncdUnit)))
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "[") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("%s: malformed line %q", xqsyncdUnit, line)
		}
		out[strings.TrimSpace(k)] = append(out[strings.TrimSpace(k)], strings.TrimSpace(v))
	}
	return out
}

// Every hardening directive spec 9.5 names is present with its value. A dropped line is caught here,
// on any OS, rather than only by systemd-analyze on the Linux runner.
func TestXqsyncdUnitCarriesTheSpecHardening(t *testing.T) {
	d := unitDirectives(t)
	want := map[string]string{
		"DynamicUser":             "yes",
		"StateDirectory":          "xqsyncd",
		"ProtectSystem":           "strict",
		"ProtectHome":             "yes",
		"PrivateTmp":              "yes",
		"NoNewPrivileges":         "yes",
		"MemoryDenyWriteExecute":  "yes",
		"RestrictAddressFamilies": "AF_INET AF_INET6 AF_UNIX",
		"SystemCallFilter":        "@system-service",
		"CapabilityBoundingSet":   "",
		"UMask":                   "0077",
		"LimitCORE":               "0",
	}
	for k, v := range want {
		got, ok := d[k]
		if !ok || len(got) != 1 || got[0] != v {
			t.Errorf("%s = %q, want exactly %q (spec 9.5)", k, got, v)
		}
	}
}

// The service initialises its own state directory on first start, as the dynamic user, and never
// replaces an existing key file.
func TestXqsyncdUnitInitialisesThenServesItsStateDirectory(t *testing.T) {
	d := unitDirectives(t)
	pre := strings.Join(d["ExecStartPre"], " ")
	if !strings.Contains(pre, "init --dir ${STATE_DIRECTORY} --keep-existing") {
		t.Errorf("ExecStartPre = %q; init must run with --keep-existing on the state directory", pre)
	}
	start := strings.Join(d["ExecStart"], " ")
	if !strings.Contains(start, "serve --dir ${STATE_DIRECTORY}") {
		t.Errorf("ExecStart = %q; serve must use the state directory", start)
	}
	if strings.HasPrefix(pre, "+") || strings.HasPrefix(pre, "!") {
		t.Error("ExecStartPre runs with full privileges; init must create the files as the service's own user")
	}
}
