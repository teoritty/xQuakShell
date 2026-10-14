//go:build windows

package main

import (
	"crypto/sha1" // #nosec G505 -- Windows derives service SIDs with SHA-1; this reproduces that, it protects nothing
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

type serviceInstall struct {
	name  string
	dir   string
	addr  string
	hosts []string
}

func parseServiceInstall(args []string) (serviceInstall, error) {
	fs := newFlags("service install")
	name := fs.String("name", "xqsyncd", "service name")
	dir := fs.String("dir", "", `data directory (default %ProgramData%\NAME)`)
	addr := fs.String("addr", ":8443", "listen address")
	var hosts hostList
	fs.Var(&hosts, "host", "host name or address for the self-signed certificate (repeatable)")
	if err := fs.Parse(args); err != nil {
		return serviceInstall{}, err
	}
	if err := validServiceName(*name); err != nil {
		return serviceInstall{}, err
	}
	o := serviceInstall{name: *name, dir: *dir, addr: *addr, hosts: hosts}
	if o.dir == "" {
		pd := os.Getenv("ProgramData")
		if pd == "" {
			return serviceInstall{}, errors.New("xqsyncd service: %ProgramData% is not set; pass --dir")
		}
		o.dir = filepath.Join(pd, o.name)
	}
	return o, nil
}

// serviceArgs is the command line the service control manager starts the server with.
func (o serviceInstall) serviceArgs() []string {
	return []string{"serve", "--dir", o.dir, "--addr", o.addr}
}

// serviceSID is the SID of the virtual account NT SERVICE\<name>, derived as Windows derives it:
// S-1-5-80 followed by the SHA-1 of the upper-cased name in UTF-16LE, read as five little-endian
// sub-authorities. Computing it means the ACL can be written before the account is ever resolved.
func serviceSID(name string) string {
	units := utf16.Encode([]rune(strings.ToUpper(name)))
	raw := make([]byte, 2*len(units))
	for i, u := range units {
		binary.LittleEndian.PutUint16(raw[2*i:], u)
	}
	sum := sha1.Sum(raw) // #nosec G401 -- the derivation Windows uses, not a security hash
	parts := []string{"S-1-5-80"}
	for i := 0; i < 5; i++ {
		parts = append(parts, strconv.FormatUint(uint64(binary.LittleEndian.Uint32(sum[4*i:])), 10))
	}
	return strings.Join(parts, "-")
}

// dataDirSDDL is the data directory's DACL: protected, so nothing is inherited from %ProgramData%
// (where every user may read), with full control for Administrators and the service account only.
// The key file is the reason: with it, a copy of the database is an offline password-guessing target.
func dataDirSDDL(serviceSID string) string {
	return "D:P(A;OICI;FA;;;BA)(A;OICI;FA;;;" + serviceSID + ")"
}

func protectDataDir(dir, serviceSID string) error {
	sd, err := windows.SecurityDescriptorFromString(dataDirSDDL(serviceSID))
	if err != nil {
		return err
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return err
	}
	return windows.SetNamedSecurityInfo(dir, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION, nil, nil, dacl, nil)
}

// installService registers the server with the service control manager under its own virtual
// account, locks the data directory down to that account and Administrators, initialises it, and
// starts it. A step that fails after the service exists removes the service again.
func installService(args []string, out io.Writer) error {
	o, err := parseServiceInstall(args)
	if err != nil {
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("xqsyncd service: connect to the service manager (run as Administrator): %w", err)
	}
	defer func() { _ = m.Disconnect() }()
	s, err := m.CreateService(o.name, exe, mgr.Config{DisplayName: "xQuakShell sync server (" + o.name + ")",
		Description: "Stores the encrypted vaults of xQuakShell sync accounts. It cannot open them.",
		StartType:   mgr.StartAutomatic, ServiceStartName: `NT SERVICE\` + o.name}, o.serviceArgs()...)
	if err != nil {
		return fmt.Errorf("xqsyncd service: create %s: %w", o.name, err)
	}
	defer func() { _ = s.Close() }()
	if err := finishInstall(o, s, out); err != nil {
		_ = s.Delete()
		_ = eventlog.Remove(o.name)
		return err
	}
	return nil
}

func finishInstall(o serviceInstall, s *mgr.Service, out io.Writer) error {
	// An event source left over from an earlier install of the same name is reused.
	if err := eventlog.InstallAsEventCreate(o.name, eventlog.Error|eventlog.Warning|eventlog.Info); err != nil &&
		!strings.Contains(err.Error(), "already exists") {
		return fmt.Errorf("xqsyncd service: event log source: %w", err)
	}
	if err := os.MkdirAll(o.dir, 0o700); err != nil {
		return err
	}
	if err := protectDataDir(o.dir, serviceSID(o.name)); err != nil {
		return fmt.Errorf("xqsyncd service: restrict %s: %w", o.dir, err)
	}
	initArgs := []string{"--dir", o.dir, "--keep-existing"}
	for _, h := range o.hosts {
		initArgs = append(initArgs, "--host", h)
	}
	if err := cmdInit(initArgs, out); err != nil {
		return err
	}
	if err := s.Start(); err != nil {
		return fmt.Errorf("xqsyncd service: start %s: %w", o.name, err)
	}
	_, err := fmt.Fprintf(out, "service %s installed and started; data in %s\n", o.name, o.dir)
	return err
}

// uninstallService stops and removes the service. The data directory stays: it holds every user's
// vaults, and deleting it is a decision the operator makes by hand.
func uninstallService(args []string, out io.Writer) error {
	fs := newFlags("service uninstall")
	name := fs.String("name", "xqsyncd", "service name")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := validServiceName(*name); err != nil {
		return err
	}
	m, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("xqsyncd service: connect to the service manager (run as Administrator): %w", err)
	}
	defer func() { _ = m.Disconnect() }()
	s, err := m.OpenService(*name)
	if err != nil {
		return fmt.Errorf("xqsyncd service: open %s: %w", *name, err)
	}
	defer func() { _ = s.Close() }()
	if err := stopAndWait(s, 30*time.Second); err != nil {
		return err
	}
	if err := s.Delete(); err != nil {
		return fmt.Errorf("xqsyncd service: delete %s: %w", *name, err)
	}
	_ = eventlog.Remove(*name)
	_, err = fmt.Fprintf(out, "service %s removed; its data directory was kept\n", *name)
	return err
}

func stopAndWait(s *mgr.Service, limit time.Duration) error {
	st, err := s.Query()
	if err != nil {
		return err
	}
	if st.State == svc.Stopped {
		return nil
	}
	if _, err := s.Control(svc.Stop); err != nil {
		return fmt.Errorf("xqsyncd service: stop: %w", err)
	}
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if st, err = s.Query(); err != nil || st.State == svc.Stopped {
			return err
		}
		time.Sleep(250 * time.Millisecond)
	}
	return errors.New("xqsyncd service: the service did not stop in time")
}
