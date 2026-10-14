//go:build windows

package main

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Windows derives a service's virtual-account SID from its name alone; TrustedInstaller's is the
// published example.
func TestServiceSIDMatchesWindows(t *testing.T) {
	const want = "S-1-5-80-956008885-3418522649-1831038044-1853292631-2271478464"
	if got := serviceSID("TrustedInstaller"); got != want {
		t.Fatalf("serviceSID(TrustedInstaller) = %s, want %s", got, want)
	}
	if serviceSID("trustedinstaller") != want {
		t.Fatal("the SID must not depend on the case of the name, as Windows upper-cases it")
	}
	if serviceSID("xqsyncd") == serviceSID("xqsyncd-e2e") {
		t.Fatal("two services share a SID")
	}
}

// The data directory's DACL is protected (nothing inherited from ProgramData, where Users can read)
// and grants only Administrators and the service account.
func TestDataDirSDDLGrantsOnlyAdministratorsAndTheService(t *testing.T) {
	sid := serviceSID("xqsyncd")
	sddl := dataDirSDDL(sid)
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		t.Fatalf("%q does not parse: %v", sddl, err)
	}
	ctl, _, err := sd.Control()
	if err != nil || ctl&windows.SE_DACL_PROTECTED == 0 {
		t.Fatalf("control %v, %v; the DACL must be protected from inheritance", ctl, err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		t.Fatal(err)
	}
	var grantees []string
	for i := uint32(0); i < uint32(dacl.AceCount); i++ {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			t.Fatal(err)
		}
		if ace.Header.AceType != windows.ACCESS_ALLOWED_ACE_TYPE {
			t.Fatalf("ACE %d is type %d; only allow entries are expected", i, ace.Header.AceType)
		}
		sidOf := (*windows.SID)(unsafe.Pointer(&ace.SidStart)) // #nosec G103 -- reading the SID the ACE carries inline, as GetAce documents
		grantees = append(grantees, sidOf.String())
	}
	slices.Sort(grantees)
	want := []string{"S-1-5-32-544", sid}
	slices.Sort(want)
	if !slices.Equal(grantees, want) {
		t.Fatalf("grantees %v, want only Administrators and the service: %v", grantees, want)
	}
}

func TestServiceInstallDefaults(t *testing.T) {
	t.Setenv("ProgramData", `C:\ProgramData`)
	o, err := parseServiceInstall([]string{})
	if err != nil {
		t.Fatal(err)
	}
	if o.name != "xqsyncd" || o.dir != filepath.Join(`C:\ProgramData`, "xqsyncd") || o.addr != ":8443" {
		t.Fatalf("defaults = %+v", o)
	}
	o, err = parseServiceInstall([]string{"--name", "xqsyncd-e2e", "--addr", "127.0.0.1:9443", "--host", "sync.example"})
	if err != nil || o.name != "xqsyncd-e2e" || o.dir != filepath.Join(`C:\ProgramData`, "xqsyncd-e2e") || !slices.Equal(o.hosts, []string{"sync.example"}) {
		t.Fatalf("parsed = %+v, %v; the default directory follows the name", o, err)
	}
	for _, bad := range []string{"", "a b", `a\b`, "a/b", strings.Repeat("x", 81)} {
		if _, err := parseServiceInstall([]string{"--name", bad}); err == nil {
			t.Errorf("service name %q accepted", bad)
		}
	}
}

func TestServiceArgsRunServeOnTheDirectory(t *testing.T) {
	o := serviceInstall{name: "xqsyncd", dir: `C:\ProgramData\xqsyncd`, addr: ":8443"}
	got := o.serviceArgs()
	want := []string{"serve", "--dir", `C:\ProgramData\xqsyncd`, "--addr", ":8443"}
	if !slices.Equal(got, want) {
		t.Fatalf("args = %q, want %q", got, want)
	}
}

func TestDataDirACLIsAppliedToADirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "data")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	// The current user's SID stands in for the service account, which does not exist in a test.
	tok := windows.GetCurrentProcessToken()
	user, err := tok.GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	if err := protectDataDir(dir, user.User.Sid.String()); err != nil {
		t.Fatalf("protect: %v", err)
	}
	sd, err := windows.GetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if ctl, _, _ := sd.Control(); ctl&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("the applied DACL is not protected")
	}
	if !strings.Contains(sd.String(), user.User.Sid.String()) {
		t.Fatalf("applied DACL %s does not name the service account", sd.String())
	}
}

// The service logs to the event source the installer registered under the service's name, which is
// also the default data directory's name.
func TestServiceNameOfFollowsTheDataDirectory(t *testing.T) {
	cases := map[string][]string{
		"xqsyncd-e2e": {"serve", "--dir", `C:\ProgramData\xqsyncd-e2e`, "--addr", ":8443"},
		"sync":        {"serve", "--dir", `D:\data\sync\`},
		"xqsyncd":     {"serve", "--dir", `C:\ProgramData\bad name`},
	}
	for want, args := range cases {
		if got := serviceNameOf(args); got != want {
			t.Errorf("serviceNameOf(%q) = %q, want %q", args, got, want)
		}
	}
	if serviceNameOf([]string{"serve"}) != "xqsyncd" {
		t.Error("without --dir the default name is used")
	}
}

func TestServiceCommandNeedsAVerb(t *testing.T) {
	for _, args := range [][]string{{"service"}, {"service", "remove"}} {
		if err := run(t.Context(), args, io.Discard); err == nil {
			t.Errorf("run(%q) succeeded", args)
		}
	}
}
