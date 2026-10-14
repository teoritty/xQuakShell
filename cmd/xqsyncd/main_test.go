package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/opaque"
	"xquakshell/internal/infra/synccrypto"
	"xquakshell/internal/syncserver/servertest"
)

func initDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "srv")
	var out bytes.Buffer
	if err := run(context.Background(), []string{"init", "--dir", dir, "--host", "localhost", "--host", "127.0.0.1"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "identity fingerprint:") {
		t.Fatalf("init did not print the fingerprint: %q", out.String())
	}
	return dir
}

func TestInitRefusesToOverwrite(t *testing.T) {
	dir := initDir(t)
	before, _ := os.ReadFile(filepath.Join(dir, keyFileName))
	err := run(context.Background(), []string{"init", "--dir", dir}, io.Discard)
	if !errors.Is(err, errKeyFileExists) {
		t.Fatalf("second init: %v, want errKeyFileExists; overwriting the key would lock every account out", err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, keyFileName))
	if !bytes.Equal(before, after) {
		t.Fatal("a refused init still changed the key file")
	}
}

func TestKeyFileIsOwnerOnly(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not apply; on Windows the file is DPAPI-sealed and the directory ACL is set by xqsyncd service install")
	}
	dir := initDir(t)
	info, err := os.Stat(filepath.Join(dir, keyFileName))
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("key file mode = %v, %v; want 0600", info.Mode().Perm(), err)
	}
}

func TestFingerprintIsStable(t *testing.T) {
	dir := initDir(t)
	var a, b bytes.Buffer
	_ = run(context.Background(), []string{"fingerprint", "--dir", dir}, &a)
	_ = run(context.Background(), []string{"fingerprint", "--dir", dir}, &b)
	first, link, _ := strings.Cut(a.String(), "\n")
	if a.String() != b.String() || len(strings.Fields(first)) != 12 {
		t.Fatalf("fingerprint %q / %q; the first line is the twelve groups scripts read", a.String(), b.String())
	}
	want := "connect link: https://localhost:8443/#fp=" + strings.ReplaceAll(first, " ", "-") + "\n"
	if link != want {
		t.Fatalf("link line %q, want %q; the link carries exactly the fingerprint above it", link, want)
	}
}

func TestFingerprintLinkNamesTheCertificateHost(t *testing.T) {
	dir := t.TempDir()
	if err := run(context.Background(), []string{"init", "--dir", dir, "--host", "sync.example.org", "--host", "10.0.0.5"}, io.Discard); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := run(context.Background(), []string{"fingerprint", "--dir", dir, "--port", "9443"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "connect link: https://sync.example.org:9443/#fp=") {
		t.Fatalf("output %q; the link names the certificate's first host and the given port", out.String())
	}
	out.Reset()
	_ = run(context.Background(), []string{"fingerprint", "--dir", dir, "--host", "10.0.0.5"}, &out)
	if !strings.Contains(out.String(), "https://10.0.0.5:8443/#fp=") {
		t.Fatalf("output %q; --host overrides the certificate", out.String())
	}
}

func TestPortOfAListeningAddress(t *testing.T) {
	for addr, want := range map[string]string{"[::]:8443": "8443", "127.0.0.1:51234": "51234", "bad": "8443"} {
		if got := portOf(addr); got != want {
			t.Errorf("portOf(%q) = %q, want %q", addr, got, want)
		}
	}
}

func TestAdminInviteAndAccounts(t *testing.T) {
	dir := initDir(t)
	var out bytes.Buffer
	if err := run(context.Background(), []string{"admin", "invite", "create", "--dir", dir, "--ttl", "1h"}, &out); err != nil {
		t.Fatal(err)
	}
	if len(strings.TrimSpace(out.String())) < 20 {
		t.Fatalf("invite code %q", out.String())
	}
	if err := run(context.Background(), []string{"admin", "account", "freeze", "--dir", dir, "nobody"}, io.Discard); err == nil {
		t.Fatal("freezing an unknown account succeeded")
	}
}

type liveServer struct {
	t      *testing.T
	dir    string
	url    string
	client *http.Client
	cancel context.CancelFunc
	done   chan error
}

func startServer(t *testing.T) *liveServer {
	t.Helper()
	dir := initDir(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	cfg := serveConfig{dir: dir, certPath: filepath.Join(dir, certFileName), keyPath: filepath.Join(dir, keyPEMName)}
	go func() { done <- serve(ctx, cfg, ln) }()
	pemBytes, _ := os.ReadFile(filepath.Join(dir, certFileName))
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(pemBytes)
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{
		TLSClientConfig: &tls.Config{RootCAs: pool, ServerName: "127.0.0.1", MinVersion: tls.VersionTLS13}}}
	s := &liveServer{t: t, dir: dir, url: "https://" + ln.Addr().String(), client: client, cancel: cancel, done: done}
	t.Cleanup(s.stop)
	return s
}

func (s *liveServer) stop() {
	s.cancel()
	select {
	case err := <-s.done:
		if err != nil {
			s.t.Errorf("serve returned %v on shutdown", err)
		}
	case <-time.After(15 * time.Second):
		s.t.Error("serve did not shut down")
	}
}

func (s *liveServer) post(path string, req, reply any) int {
	s.t.Helper()
	raw, _ := json.Marshal(req)
	resp, err := s.client.Post(s.url+path, "application/json", bytes.NewReader(raw))
	if err != nil {
		s.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	if reply != nil && resp.StatusCode == http.StatusOK {
		if err := json.Unmarshal(body, reply); err != nil {
			s.t.Fatalf("%s: %v (%s)", path, err, body)
		}
	}
	return resp.StatusCode
}

// TestEndToEndWithRealOPAQUE runs the real server over TLS and registers and logs in with the real
// OPAQUE client at full suite cost, then opens the bundle with the export key the login produced.
// It is the one test where every cryptographic piece of the account flow runs for real.
func TestEndToEndWithRealOPAQUE(t *testing.T) {
	s := startServer(t)
	var hello syncwire.HelloReply
	if s.post("/v1/hello", syncwire.Hello{ClientProtocol: syncwire.Version{Major: 1}, Suite: 1}, &hello) != http.StatusOK {
		t.Fatal("hello failed")
	}
	k, _ := readKeyFile(s.dir)
	if !bytes.Equal(hello.ServerIdentityKey, ed25519.NewKeyFromSeed(k.IdentitySeed).Public().(ed25519.PublicKey)) {
		t.Fatal("hello serves a different identity key than the key file holds")
	}
	helloCtx, _ := vaultsync.HelloContext(hello.ServerIdentityKey, vaultsync.CurrentProtocol, vaultsync.CurrentProtocol, []vaultsync.SuiteID{vaultsync.Suite1})
	var invite bytes.Buffer
	if err := run(context.Background(), []string{"admin", "invite", "create", "--dir", s.dir}, &invite); err != nil {
		t.Fatal(err)
	}
	inv := strings.TrimSpace(invite.String())
	u := servertest.NewUser(t, "alice")
	exportKey := s.register(t, helloCtx, inv, u)
	loginExport, bundle := s.login(t, helloCtx, u)
	if !bytes.Equal(exportKey, loginExport) {
		t.Fatal("the export key at login differs from registration; the bundle would never open")
	}
	keys, err := synccrypto.OpenBundle(loginExport, u.Account, bundle)
	if err != nil || !bytes.Equal(keys.IdentitySeed, u.AIK.Seed()) {
		t.Fatalf("the bundle from the server does not open with the login's export key: %v", err)
	}
}

func (s *liveServer) register(t *testing.T, helloCtx []byte, invite string, u *servertest.User) []byte {
	cl, _ := opaque.NewClient(helloCtx)
	req, _ := cl.RegistrationStart([]byte("correct horse battery staple"))
	var init syncwire.RegisterInitReply
	if st := s.post("/v1/register/init", syncwire.RegisterInit{Invite: invite, Username: u.Name, Account: u.Account.String(), OPAQUE: req}, &init); st != http.StatusOK {
		t.Fatalf("register init: %d", st)
	}
	record, exportKey, err := cl.RegistrationFinish(init.OPAQUE, u.Account)
	if err != nil {
		t.Fatal(err)
	}
	bundle, _ := synccrypto.SealBundle(rand.Reader, exportKey, u.Account, synccrypto.AccountKeys{IdentitySeed: u.AIK.Seed(), EncryptionSeed: make([]byte, 32)})
	dev := u.AddDeviceEntry(t, u.Device, u.DSK)
	create := u.CreateOp(t, u.Vault)
	fin := syncwire.RegisterFinish{Invite: invite, Username: u.Name, Account: u.Account.String(), Record: record,
		IdentityKey: u.AIK.Public().(ed25519.PublicKey), EncryptionKey: u.AEK, Bundle: bundle,
		Device: syncwire.DeviceEntryFrom(&dev), Vault: syncwire.MemberOpFrom(&create)}
	if st := s.post("/v1/register/finish", fin, nil); st != http.StatusOK {
		t.Fatalf("register finish: %d", st)
	}
	return exportKey
}

func (s *liveServer) login(t *testing.T, helloCtx []byte, u *servertest.User) ([]byte, []byte) {
	cl, _ := opaque.NewClient(helloCtx)
	ke1, _ := cl.LoginStart([]byte("correct horse battery staple"))
	var li syncwire.LoginInitReply
	if st := s.post("/v1/login/init", syncwire.LoginInit{Username: u.Name, KE1: ke1}, &li); st != http.StatusOK {
		t.Fatalf("login init: %d", st)
	}
	ke3, _, exportKey, err := cl.LoginFinish(li.KE2, u.Account)
	if err != nil {
		t.Fatalf("client could not finish a correct login: %v", err)
	}
	var lf syncwire.LoginFinishReply
	if st := s.post("/v1/login/finish", syncwire.LoginFinish{LoginID: li.LoginID, KE3: ke3}, &lf); st != http.StatusOK {
		t.Fatalf("login finish: %d", st)
	}
	return exportKey, lf.Bundle
}
