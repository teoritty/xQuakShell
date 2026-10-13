package wails

import (
	"context"
	"encoding/hex"
	"errors"
	"strings"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/usecase"
)

type fakeSync struct {
	started, stopped int
	cred             usecase.SyncCredentials
	registered       bool
	joinNeeded       bool
	join             usecase.JoinChoice
	revoked          []vaultsync.ID
	confirmed        vaultsync.ID
	masterPassword   string
	err              error
	info             *usecase.SyncAccountInfo
}

func (f *fakeSync) Start(context.Context) error { f.started++; return f.err }
func (f *fakeSync) Stop()                       { f.stopped++ }
func (f *fakeSync) Status() (usecase.SyncStatus, bool) {
	return usecase.SyncStatus{State: usecase.SyncIdle, Report: usecase.SyncReport{Pull: usecase.PullReport{Held: 2}}}, true
}
func (f *fakeSync) AccountInfo() (usecase.SyncAccountInfo, bool, error) {
	if f.info == nil {
		return usecase.SyncAccountInfo{}, false, nil
	}
	return *f.info, true, nil
}
func (f *fakeSync) Probe(context.Context, string) (usecase.ServerProbe, error) {
	return usecase.ServerProbe{IdentityKey: make([]byte, 32), Fingerprint: "00001 00002"}, f.err
}
func (f *fakeSync) Register(_ context.Context, c usecase.SyncCredentials) error {
	f.cred, f.registered = c, true
	return f.err
}
func (f *fakeSync) Login(_ context.Context, c usecase.SyncCredentials) (bool, error) {
	f.cred = c
	return f.joinNeeded, f.err
}
func (f *fakeSync) Join(_ context.Context, c usecase.JoinChoice) (string, error) {
	f.join = c
	return "vault.age.sync-join-1.bak", f.err
}
func (f *fakeSync) Relogin(context.Context, []byte) error                { return f.err }
func (f *fakeSync) ChangePassword(context.Context, []byte, []byte) error { return f.err }
func (f *fakeSync) ResetPassword(context.Context, []byte) error          { return f.err }
func (f *fakeSync) RevokeDevices(_ context.Context, ids []vaultsync.ID, _ []byte) error {
	f.revoked = ids
	return f.err
}
func (f *fakeSync) Disable(context.Context) error { return f.err }
func (f *fakeSync) SyncNow() error                { return f.err }
func (f *fakeSync) Devices(context.Context) ([]usecase.DeviceInfo, error) {
	return []usecase.DeviceInfo{{Cert: vaultsync.DeviceCert{Device: vaultsync.ID{1}, Name: "phone"}, Current: true}}, f.err
}
func (f *fakeSync) HeldChanges(context.Context) ([]usecase.HeldChange, error) {
	return []usecase.HeldChange{{Record: vaultsync.ID{2}, Type: "settings.plugins", Pending: true}}, f.err
}
func (f *fakeSync) ConfirmHeld(_ context.Context, id vaultsync.ID, pw string) error {
	f.confirmed, f.masterPassword = id, pw
	return f.err
}
func (f *fakeSync) RejectHeld(context.Context, vaultsync.ID) error { return f.err }

type fakeConflicts struct {
	list     []domain.SyncConflict
	resolved string
	useOther bool
}

func (f *fakeConflicts) List() ([]domain.SyncConflict, error) { return f.list, nil }
func (f *fakeConflicts) Resolve(_ context.Context, id string, useOther bool) error {
	f.resolved, f.useOther = id, useOther
	return nil
}

var _ SyncCommands = (*fakeSync)(nil)

func syncAPI() (*AppAPI, *fakeSync, *fakeConflicts) {
	a := &AppAPI{}
	f, c := &fakeSync{}, &fakeConflicts{}
	a.SetSyncServices(f, c)
	return a, f, c
}

var pinHex = hex.EncodeToString(make([]byte, 32))

func TestSyncHandlersWithoutSync(t *testing.T) {
	a := &AppAPI{}
	if st, err := a.GetSyncStatus(); err != nil || st.State != "off" || st.Enabled {
		t.Fatalf("status = %+v, %v", st, err)
	}
	for name, err := range map[string]error{
		"probe":   func() error { _, err := a.ProbeSyncServer("https://x"); return err }(),
		"enable":  func() error { _, err := a.EnableSync(EnableSyncRequest{}); return err }(),
		"devices": func() error { _, err := a.ListSyncDevices(); return err }(),
		"held":    func() error { _, err := a.ListSyncHeld(); return err }(),
		"confirm": a.ConfirmSyncHeld("00", "pw"),
		"now":     a.SyncNow(),
	} {
		if !errors.Is(err, errSyncUnavailable) {
			t.Errorf("%s: %v", name, err)
		}
	}
	a.startSync()
	a.stopSync()
}

func TestEnableSyncValidatesBeforeAnythingIsSent(t *testing.T) {
	a, f, _ := syncAPI()
	good := EnableSyncRequest{ServerURL: "https://sync.example", IdentityKey: pinHex, Username: "alice", Password: "pw", Invite: "inv", Register: true}
	for name, edit := range map[string]func(*EnableSyncRequest){
		"plain http":      func(r *EnableSyncRequest) { r.ServerURL = "http://sync.example" },
		"no host":         func(r *EnableSyncRequest) { r.ServerURL = "https://" },
		"short pin":       func(r *EnableSyncRequest) { r.IdentityKey = "abcd" },
		"not hex":         func(r *EnableSyncRequest) { r.IdentityKey = strings.Repeat("z", 64) },
		"no username":     func(r *EnableSyncRequest) { r.Username = "  " },
		"no password":     func(r *EnableSyncRequest) { r.Password = "" },
		"register no inv": func(r *EnableSyncRequest) { r.Invite = "" },
	} {
		req := good
		edit(&req)
		if _, err := a.EnableSync(req); !errors.Is(err, errInvalidSyncRequest) {
			t.Errorf("%s: %v", name, err)
		}
	}
	if f.registered {
		t.Fatal("an invalid request reached the controller")
	}
	if _, err := a.EnableSync(good); err != nil || !f.registered || string(f.cred.Password) != "pw" || len(f.cred.Pin) != 32 {
		t.Fatalf("register: %v, cred %+v", err, f.cred)
	}
	f.joinNeeded = true
	login := good
	login.Register, login.Invite = false, ""
	if res, err := a.EnableSync(login); err != nil || !res.JoinNeeded {
		t.Fatalf("login = %+v, %v", res, err)
	}
}

func TestJoinSyncTakesOnlyTheTwoChoices(t *testing.T) {
	a, f, _ := syncAPI()
	if _, err := a.JoinSync("delete-everything"); !errors.Is(err, errInvalidSyncRequest) {
		t.Fatalf("err = %v", err)
	}
	if name, err := a.JoinSync("replace"); err != nil || f.join != usecase.JoinReplace || name == "" {
		t.Fatalf("join = %q, %v, %v", name, err, f.join)
	}
}

func TestRecordAndDeviceIDsAreValidated(t *testing.T) {
	a, f, _ := syncAPI()
	for _, bad := range []string{"", "xyz", "00", strings.Repeat("0", 32)} {
		if err := a.ConfirmSyncHeld(bad, "pw"); !errors.Is(err, errInvalidSyncRequest) {
			t.Errorf("confirm %q: %v", bad, err)
		}
		if err := a.RevokeSyncDevice(bad, "pw"); !errors.Is(err, errInvalidSyncRequest) {
			t.Errorf("revoke %q: %v", bad, err)
		}
	}
	id := hex.EncodeToString((&vaultsync.ID{2})[:])
	if err := a.ConfirmSyncHeld(id, "master"); err != nil || f.confirmed != (vaultsync.ID{2}) || f.masterPassword != "master" {
		t.Fatalf("confirm: %v", err)
	}
	if err := a.RevokeSyncDevice(id, ""); !errors.Is(err, errInvalidSyncRequest) {
		t.Fatalf("revoke without the password: %v", err)
	}
	if err := a.RevokeSyncDevice(id, "pw"); err != nil || len(f.revoked) != 1 {
		t.Fatalf("revoke: %v", err)
	}
}

func TestSyncErrorsReachTheUserAsFixedSentences(t *testing.T) {
	a, f, _ := syncAPI()
	f.err = synctrust.ErrIntegrity
	err := a.SyncNow()
	if err == nil || !strings.Contains(err.Error(), "failed verification") || !errors.Is(err, synctrust.ErrIntegrity) {
		t.Fatalf("err = %v", err)
	}
	f.err = errors.New(`sql: table accounts at C:\Users\x\state.db`)
	if err := a.SyncNow(); err == nil || strings.Contains(err.Error(), "state.db") || strings.Contains(err.Error(), "sql") {
		t.Fatalf("err = %v; internal text reached the UI", err)
	}
	f.err = &synctrust.RateLimitedError{}
	if err := a.SyncNow(); err == nil || !strings.Contains(err.Error(), "Too many") {
		t.Fatalf("err = %v", err)
	}
	f.err = usecase.ErrHeldReauth
	if err := a.ConfirmSyncHeld(hex.EncodeToString((&vaultsync.ID{2})[:]), "wrong"); err == nil || !strings.Contains(err.Error(), "master password") {
		t.Fatalf("err = %v", err)
	}
}

func TestSecretConflictValuesAreNotSent(t *testing.T) {
	a, _, c := syncAPI()
	c.list = []domain.SyncConflict{
		{ID: "p", RecordType: "password", ItemKey: "p1", Path: []string{"value"}, Local: []byte(`"c2VjcmV0"`), Remote: []byte(`"b3RoZXI="`)},
		{ID: "h", RecordType: "connection", ItemKey: "c1", Path: []string{"host"}, Local: []byte(`"a.example"`), Remote: []byte(`"b.example"`)},
		{ID: "k", RecordType: "identity", ItemKey: "k1", Path: []string{"blob", "pemData"}, Local: []byte(`"x"`)},
	}
	list, err := a.ListSyncConflicts()
	if err != nil || len(list) != 3 {
		t.Fatalf("list = %+v, %v", list, err)
	}
	for _, dto := range list {
		secret := dto.ID != "h"
		if dto.Secret != secret || (secret && (dto.Local != "" || dto.Remote != "")) {
			t.Errorf("%s: %+v; a secret value must never be sent to the WebView", dto.ID, dto)
		}
	}
	if list[1].Local != `"a.example"` {
		t.Fatal("a non-secret value was withheld")
	}
	if err := a.ResolveSyncConflict("", true); !errors.Is(err, errInvalidSyncRequest) {
		t.Fatalf("empty id: %v", err)
	}
	if err := a.ResolveSyncConflict("h", true); err != nil || c.resolved != "h" || !c.useOther {
		t.Fatalf("resolve: %v", err)
	}
}

func TestSyncStatusAndLifecycleHooks(t *testing.T) {
	a, f, _ := syncAPI()
	f.info = &usecase.SyncAccountInfo{ServerURL: "https://s", Username: "alice", ServerIdentity: make([]byte, 32), JoinPending: true}
	st, err := a.GetSyncStatus()
	if err != nil || !st.Enabled || st.State != "idle" || st.Held != 2 || st.Fingerprint == "" || st.Username != "alice" || !st.JoinPending {
		t.Fatalf("status = %+v, %v", st, err)
	}
	a.startSync()
	a.stopSync()
	if f.started != 1 || f.stopped != 1 {
		t.Fatalf("started %d, stopped %d", f.started, f.stopped)
	}
	f.err = errors.New("server down")
	a.startSync()
}

func TestDevicesAndHeldAreMapped(t *testing.T) {
	a, _, _ := syncAPI()
	devices, err := a.ListSyncDevices()
	if err != nil || len(devices) != 1 || devices[0].Name != "phone" || !devices[0].Current || len(devices[0].ID) != 32 {
		t.Fatalf("devices = %+v, %v", devices, err)
	}
	held, err := a.ListSyncHeld()
	if err != nil || len(held) != 1 || held[0].Type != "settings.plugins" || !held[0].Pending {
		t.Fatalf("held = %+v, %v", held, err)
	}
}
