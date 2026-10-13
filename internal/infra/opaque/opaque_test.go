package opaque

import (
	"bytes"
	"errors"
	"slices"
	"testing"

	"xquakshell/internal/domain/vaultsync"
)

var (
	account = vaultsync.ID{0xa1}
	ctxA    = []byte("hello transcript A")
	ctxB    = []byte("hello transcript B")
	// fastKSF keeps the suite fast in tests; TestSuiteKSFParametersAreThoseOfSuite1 proves the
	// constructors callers use never take it.
	fastKSF = ksfSettings{params: []uint64{1, 64, 1}, length: 32}
)

func newPair(t *testing.T, clientCtx, serverCtx []byte) (*Client, *Server) {
	t.Helper()
	skm, err := GenerateServerKeyMaterial()
	if err != nil {
		t.Fatal(err)
	}
	srv, err := newServer(serverCtx, skm)
	if err != nil {
		t.Fatal(err)
	}
	cl, err := newClient(clientCtx, fastKSF)
	if err != nil {
		t.Fatal(err)
	}
	return cl, srv
}

func register(t *testing.T, cl *Client, srv *Server, password string) (record, exportKey []byte) {
	t.Helper()
	req, err := cl.RegistrationStart([]byte(password))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := srv.RegistrationResponse(req, account)
	if err != nil {
		t.Fatal(err)
	}
	record, exportKey, err = cl.RegistrationFinish(resp, account)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.ValidateRecord(record); err != nil {
		t.Fatalf("server refused a record its own registration produced: %v", err)
	}
	return record, exportKey
}

func TestSuiteKSFParametersAreThoseOfSuite1(t *testing.T) {
	got := suite1KSF()
	if !slices.Equal(got.params, []uint64{3, 262144, 4}) || got.length != 32 {
		t.Fatalf("suite KSF = %+v, want time 3, 256 MiB, 4 threads, 32 bytes; the library default is 64 MiB", got)
	}
	cl, err := NewClient(ctxA)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(cl.ksf.params, got.params) || cl.ksf.length != got.length {
		t.Fatal("NewClient does not use the suite 1 KSF; a forgotten parameter silently weakens every password")
	}
}

func TestRegisterThenLogin(t *testing.T) {
	cl, srv := newPair(t, ctxA, ctxA)
	record, regExport := register(t, cl, srv, "correct horse")

	ke1, err := cl.LoginStart([]byte("correct horse"))
	if err != nil {
		t.Fatal(err)
	}
	ke2, mac, serverSession, err := srv.LoginStart(ke1, record, account)
	if err != nil {
		t.Fatal(err)
	}
	ke3, clientSession, loginExport, err := cl.LoginFinish(ke2, account)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.LoginFinish(ke3, mac); err != nil {
		t.Fatalf("server refused a correct login: %v", err)
	}
	if !bytes.Equal(clientSession, serverSession) || len(clientSession) == 0 {
		t.Fatal("client and server session keys differ")
	}
	if !bytes.Equal(regExport, loginExport) || len(loginExport) == 0 {
		t.Fatal("the export key changed between registration and login; the bundle sealed under it would never open")
	}
}

func TestWrongPasswordFails(t *testing.T) {
	cl, srv := newPair(t, ctxA, ctxA)
	record, _ := register(t, cl, srv, "correct horse")
	ke1, _ := cl.LoginStart([]byte("wrong horse"))
	ke2, _, _, err := srv.LoginStart(ke1, record, account)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := cl.LoginFinish(ke2, account); !errors.Is(err, vaultsync.ErrAuthFailed) {
		t.Fatalf("client err = %v, want ErrAuthFailed", err)
	}
}

func TestServerRefusesForgedKE3(t *testing.T) {
	cl, srv := newPair(t, ctxA, ctxA)
	record, _ := register(t, cl, srv, "correct horse")
	ke1, _ := cl.LoginStart([]byte("correct horse"))
	ke2, mac, _, _ := srv.LoginStart(ke1, record, account)
	ke3, _, _, err := cl.LoginFinish(ke2, account)
	if err != nil {
		t.Fatal(err)
	}
	ke3[len(ke3)-1] ^= 1
	if err := srv.LoginFinish(ke3, mac); !errors.Is(err, vaultsync.ErrAuthFailed) {
		t.Fatalf("err = %v, want ErrAuthFailed for a tampered client MAC", err)
	}
}

func TestUnknownUserLooksLikeWrongPassword(t *testing.T) {
	cl, srv := newPair(t, ctxA, ctxA)
	record, _ := register(t, cl, srv, "correct horse")
	ke1, _ := cl.LoginStart([]byte("anything"))
	real, _, _, _ := srv.LoginStart(ke1, record, account)

	cl2, _ := newClient(ctxA, fastKSF)
	ke1b, _ := cl2.LoginStart([]byte("anything"))
	fake, err := srv.LoginStartUnknown(ke1b, vaultsync.ID{0xee})
	if err != nil {
		t.Fatal(err)
	}
	if len(fake) != len(real) {
		t.Fatalf("fake KE2 is %d bytes, real is %d; the size alone would reveal which accounts exist", len(fake), len(real))
	}
	if _, _, _, err := cl2.LoginFinish(fake, vaultsync.ID{0xee}); !errors.Is(err, vaultsync.ErrAuthFailed) {
		t.Fatalf("client err = %v, want the same ErrAuthFailed as a wrong password", err)
	}
}

func TestLoginFailsWhenContextsDiffer(t *testing.T) {
	skm, _ := GenerateServerKeyMaterial()
	srvA, _ := newServer(ctxA, skm)
	srvB, _ := newServer(ctxB, skm)
	cl, _ := newClient(ctxA, fastKSF)
	record, _ := register(t, cl, srvA, "correct horse")

	ke1, _ := cl.LoginStart([]byte("correct horse"))
	ke2, _, _, err := srvB.LoginStart(ke1, record, account)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := cl.LoginFinish(ke2, account); !errors.Is(err, vaultsync.ErrAuthFailed) {
		t.Fatalf("err = %v, want ErrAuthFailed; differing hello transcripts must break the login (downgrade protection)", err)
	}
}

func TestLoginFailsForOtherAccountIdentity(t *testing.T) {
	cl, srv := newPair(t, ctxA, ctxA)
	record, _ := register(t, cl, srv, "correct horse")
	ke1, _ := cl.LoginStart([]byte("correct horse"))
	ke2, _, _, _ := srv.LoginStart(ke1, record, account)
	if _, _, _, err := cl.LoginFinish(ke2, vaultsync.ID{0xa2}); !errors.Is(err, vaultsync.ErrAuthFailed) {
		t.Fatalf("err = %v, want ErrAuthFailed; a record must only authenticate the account it was made for", err)
	}
}

func TestRecordDoesNotContainPassword(t *testing.T) {
	cl, srv := newPair(t, ctxA, ctxA)
	record, _ := register(t, cl, srv, "a very distinctive password")
	if bytes.Contains(record, []byte("a very distinctive password")) {
		t.Fatal("the stored record contains the password")
	}
}

func TestMalformedMessagesRefused(t *testing.T) {
	cl, srv := newPair(t, ctxA, ctxA)
	record, _ := register(t, cl, srv, "pw")
	if _, err := srv.RegistrationResponse([]byte{1, 2, 3}, account); !errors.Is(err, vaultsync.ErrMalformedMessage) {
		t.Errorf("short registration request: err = %v", err)
	}
	if err := srv.ValidateRecord([]byte{1}); !errors.Is(err, vaultsync.ErrMalformedMessage) {
		t.Errorf("short record: err = %v", err)
	}
	if _, _, _, err := srv.LoginStart([]byte{1}, record, account); !errors.Is(err, vaultsync.ErrMalformedMessage) {
		t.Errorf("short KE1: err = %v", err)
	}
	ke1, _ := cl.LoginStart([]byte("pw"))
	if _, _, _, err := cl.LoginFinish([]byte{1}, account); !errors.Is(err, vaultsync.ErrMalformedMessage) {
		t.Errorf("short KE2: err = %v", err)
	}
	_, mac, _, _ := srv.LoginStart(ke1, record, account)
	if err := srv.LoginFinish([]byte{1}, mac); !errors.Is(err, vaultsync.ErrMalformedMessage) {
		t.Errorf("short KE3: err = %v", err)
	}
}

func TestServerKeyMaterialRoundTrip(t *testing.T) {
	skm, err := GenerateServerKeyMaterial()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newServer(ctxA, skm); err != nil {
		t.Fatalf("server refused key material it generated: %v", err)
	}
	if _, err := newServer(ctxA, skm[:len(skm)-1]); err == nil {
		t.Fatal("server accepted truncated key material")
	}
}

func FuzzServerLoginStart(f *testing.F) {
	skm, _ := GenerateServerKeyMaterial()
	srv, _ := newServer(ctxA, skm)
	f.Add([]byte{})
	f.Add(make([]byte, 96))
	f.Fuzz(func(t *testing.T, ke1 []byte) {
		_, _ = srv.LoginStartUnknown(ke1, account)
	})
}

// The parameters must reach the library, not merely sit in the adapter: a login whose stretching
// differs from the registration's has to fail. If they were dropped, both sides would silently use
// the library default and this login would succeed.
func TestKSFParametersReachTheLibrary(t *testing.T) {
	skm, _ := GenerateServerKeyMaterial()
	srv, _ := newServer(ctxA, skm)
	registrar, _ := newClient(ctxA, fastKSF)
	record, _ := register(t, registrar, srv, "correct horse")

	other, _ := newClient(ctxA, ksfSettings{params: []uint64{2, 64, 1}, length: 32})
	ke1, _ := other.LoginStart([]byte("correct horse"))
	ke2, _, _, err := srv.LoginStart(ke1, record, account)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := other.LoginFinish(ke2, account); !errors.Is(err, vaultsync.ErrAuthFailed) {
		t.Fatalf("err = %v; a login with different stretching parameters succeeded, so the parameters are not being applied", err)
	}
}

// One full exchange at the real suite 1 cost, so the production path is exercised at least once.
func TestProductionSuiteLogin(t *testing.T) {
	skm, _ := GenerateServerKeyMaterial()
	srv, _ := NewServer(ctxA, skm)
	cl, err := NewClient(ctxA)
	if err != nil {
		t.Fatal(err)
	}
	record, regExport := register(t, cl, srv, "correct horse")
	cl2, _ := NewClient(ctxA)
	ke1, _ := cl2.LoginStart([]byte("correct horse"))
	ke2, mac, _, _ := srv.LoginStart(ke1, record, account)
	ke3, _, loginExport, err := cl2.LoginFinish(ke2, account)
	if err != nil || srv.LoginFinish(ke3, mac) != nil || string(regExport) != string(loginExport) {
		t.Fatalf("suite 1 login failed: %v", err)
	}
}
