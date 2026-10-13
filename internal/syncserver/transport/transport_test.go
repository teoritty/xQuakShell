package transport_test

import (
	"bytes"
	"crypto/ed25519"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/servertest"
	"xquakshell/internal/syncserver/transport"
)

var identitySeed = bytes.Repeat([]byte{0x5e}, 32)

type rig struct {
	t   *testing.T
	h   *servertest.Harness
	srv *httptest.Server
}

func newRig(t *testing.T) *rig {
	t.Helper()
	h := servertest.NewHarness(t, 0)
	handler, err := transport.New(transport.Config{IdentitySeed: identitySeed, TLSPublicKey: []byte("spki"),
		MaxBody: 64 << 10, RatePerSecond: 10_000, Burst: 10_000}, h.Svc)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return &rig{t: t, h: h, srv: srv}
}

// do sends a request and returns the status and raw body.
func (r *rig) do(method, path, token string, body any) (int, []byte) {
	r.t.Helper()
	var rd io.Reader
	switch b := body.(type) {
	case nil:
	case string:
		rd = strings.NewReader(b)
	default:
		raw, err := json.Marshal(b)
		if err != nil {
			r.t.Fatal(err)
		}
		rd = bytes.NewReader(raw)
	}
	req, _ := http.NewRequest(method, r.srv.URL+path, rd)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		r.t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw
}

var errorBody = regexp.MustCompile(`^\{"error":"(malformed|unauthorized|forbidden|not_found|conflict|too_large|rate_limited|unsupported_version|frozen|quota_exceeded|internal)"(,"currentSeq":\d+)?\}\n$`)

// assertErrorBody holds every error response to a bare code, never Go error text.
func assertErrorBody(t *testing.T, status int, body []byte) {
	t.Helper()
	if status >= 400 && !errorBody.Match(body) {
		t.Fatalf("error body %q is not a bare code from the closed list", body)
	}
}

func TestHelloSignsTheTLSKey(t *testing.T) {
	r := newRig(t)
	status, body := r.do("POST", "/v1/hello", "", syncwire.Hello{ClientProtocol: syncwire.Version{Major: 1}, Suite: 1})
	if status != http.StatusOK {
		t.Fatalf("hello status %d: %s", status, body)
	}
	var reply syncwire.HelloReply
	if err := json.Unmarshal(body, &reply); err != nil {
		t.Fatal(err)
	}
	msg, _ := vaultsync.ServerTLSMessage([]byte("spki"))
	want := ed25519.NewKeyFromSeed(identitySeed).Public().(ed25519.PublicKey)
	if !bytes.Equal(reply.ServerIdentityKey, want) || !ed25519.Verify(want, msg, reply.TLSKeySig) {
		t.Fatal("hello does not carry a valid identity signature over the TLS key; the client could not pin the identity")
	}
	status, body = r.do("POST", "/v1/hello", "", syncwire.Hello{ClientProtocol: syncwire.Version{Major: 2}, Suite: 1})
	if status != http.StatusBadRequest || !strings.Contains(string(body), "unsupported_version") {
		t.Fatalf("a client of another major: %d %s", status, body)
	}
}

func TestStrictDecoding(t *testing.T) {
	r := newRig(t)
	status, body := r.do("POST", "/v1/hello", "", `{"suite":1,"clientProtocol":{"major":1},"extra":1}`)
	if status != http.StatusBadRequest {
		t.Fatalf("unknown field: %d", status)
	}
	assertErrorBody(t, status, body)
	status, body = r.do("POST", "/v1/hello", "", `{"suite":1}`+strings.Repeat(" ", 70<<10))
	if status != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize body: %d %s", status, body)
	}
	assertErrorBody(t, status, body)
	status, body = r.do("GET", "/v1/vaults/NOT-AN-ID/changes", "x", nil)
	assertErrorBody(t, status, body)
}

func TestFullFlowOverHTTP(t *testing.T) {
	r := newRig(t)
	u := servertest.NewUser(t, "alice")
	inv := r.h.Invite()
	status, body := r.do("POST", "/v1/register/init", "", syncwire.RegisterInit{Invite: inv, Username: "alice", Account: u.Account.String(), OPAQUE: []byte("b")})
	if status != http.StatusOK {
		t.Fatalf("register init %d %s", status, body)
	}
	reg := r.h.Registration(u, inv)
	status, body = r.do("POST", "/v1/register/finish", "", syncwire.RegisterFinish{Invite: inv, Username: "alice",
		Account: u.Account.String(), Record: reg.Record, IdentityKey: reg.IdentityKey, EncryptionKey: reg.EncryptionKey,
		Bundle: reg.Bundle, Device: syncwire.DeviceEntryFrom(&reg.Device), Vault: syncwire.MemberOpFrom(&reg.Vault)})
	if status != http.StatusOK {
		t.Fatalf("register finish %d %s", status, body)
	}
	var li syncwire.LoginInitReply
	_, body = r.do("POST", "/v1/login/init", "", syncwire.LoginInit{Username: "alice", KE1: []byte("k")})
	_ = json.Unmarshal(body, &li)
	status, body = r.do("POST", "/v1/login/finish", "", syncwire.LoginFinish{LoginID: li.LoginID, KE3: servertest.CorrectKE3([]byte("k"), u.Record)})
	var lf syncwire.LoginFinishReply
	if status != http.StatusOK || json.Unmarshal(body, &lf) != nil || string(lf.Bundle) != "sealed-bundle" {
		t.Fatalf("login finish %d %s", status, body)
	}
	var ch syncwire.DeviceChallengeReply
	_, body = r.do("POST", "/v1/device/challenge", "", syncwire.DeviceChallenge{Account: u.Account.String(), Device: u.Device.String()})
	_ = json.Unmarshal(body, &ch)
	msg, _ := vaultsync.DeviceAuthMessage(u.Account, u.Device, ch.Nonce)
	var acc syncwire.AccessReply
	status, body = r.do("POST", "/v1/device/verify", "", syncwire.DeviceVerify{Account: u.Account.String(), Device: u.Device.String(), Nonce: ch.Nonce, Sig: ed25519.Sign(u.DSK, msg)})
	if status != http.StatusOK || json.Unmarshal(body, &acc) != nil {
		t.Fatalf("device verify %d %s", status, body)
	}
	u.Access = acc.AccessToken
	cv := r.h.PersonalVault(u)
	c := cv.Commit(u, cv.Record(u, 1))
	wire := syncwire.Commit{ExpectSeq: 0, Head: syncwire.HeadFrom(&c.Head), Records: []syncwire.Envelope{syncwire.EnvelopeFrom(&c.Records[0])}}
	status, body = r.do("POST", "/v1/vaults/"+u.Vault.String()+"/commit", u.Access, wire)
	if status != http.StatusOK || !strings.Contains(string(body), `"seq":1`) {
		t.Fatalf("commit %d %s", status, body)
	}
	status, body = r.do("POST", "/v1/vaults/"+u.Vault.String()+"/commit", u.Access, wire)
	if status != http.StatusOK || !strings.Contains(string(body), `"seq":1`) {
		t.Fatalf("a retried commit over HTTP: %d %s", status, body)
	}
	var changes syncwire.Changes
	status, body = r.do("GET", "/v1/vaults/"+u.Vault.String()+"/changes?since=0", u.Access, nil)
	if status != http.StatusOK || json.Unmarshal(body, &changes) != nil || len(changes.Commits) != 1 {
		t.Fatalf("changes %d %s", status, body)
	}
	got, err := changes.Commits[0].Head.ToDomain()
	if err != nil || got.MerkleRoot != c.Head.MerkleRoot {
		t.Fatal("the head served back differs from the one committed")
	}
}

func TestStaleCommitReturnsConflictWithCurrentSeq(t *testing.T) {
	r := newRig(t)
	u := r.h.Register("alice")
	cv := r.h.PersonalVault(u)
	cv.Accept(u, cv.Commit(u, cv.Record(u, 1)))
	stale, _ := vaultsync.NewVaultState(u.Vault)
	cv2 := *cv
	cv2.State = stale
	c := cv2.Commit(u, cv2.Record(u, 2))
	wire := syncwire.Commit{ExpectSeq: 0, Head: syncwire.HeadFrom(&c.Head), Records: []syncwire.Envelope{syncwire.EnvelopeFrom(&c.Records[0])}}
	status, body := r.do("POST", "/v1/vaults/"+u.Vault.String()+"/commit", u.Access, wire)
	if status != http.StatusConflict || !strings.Contains(string(body), `"currentSeq":1`) {
		t.Fatalf("stale commit: %d %s", status, body)
	}
	assertErrorBody(t, status, body)
}

// idorRig is a team vault with every kind of caller the matrix probes.
type idorRig struct {
	*rig
	cv      *servertest.ClientVault
	callers map[string]*servertest.User
	tokens  map[string]string
}

func newIDORRig(t *testing.T) *idorRig {
	r := newRig(t)
	h := r.h
	alice, bob, carol := h.Register("alice"), h.Register("bob"), h.Register("carol")
	dave, erin, frank := h.Register("dave"), h.Register("erin"), h.Register("frank")
	gina := h.Register("gina")
	cv := h.NewTeamVault(alice,
		servertest.Member{User: bob, Role: vaultsync.RoleEditor}, servertest.Member{User: carol, Role: vaultsync.RoleViewer},
		servertest.Member{User: erin, Role: vaultsync.RoleEditor}, servertest.Member{User: frank, Role: vaultsync.RoleEditor},
		servertest.Member{User: gina, Role: vaultsync.RoleEditor})
	cv.Users[dave.Account] = dave
	if err := cv.AppendOps(alice, vaultsync.MemberOp{Kind: vaultsync.MemberRemove, Subject: erin.Account},
		vaultsync.MemberOp{Kind: vaultsync.MemberRotateEpoch, Epoch: 1,
			Wraps: servertest.WrapFor(alice.Account, bob.Account, carol.Account, frank.Account, gina.Account)}); err != nil {
		t.Fatal(err)
	}
	ginaRevoked := revokedSecondDevice(t, h, gina)
	if err := h.Svc.SetFrozen(servertest.Ctx, "frank", true); err != nil {
		t.Fatal(err)
	}
	id, _, _, _ := h.Svc.LoginStart(servertest.Ctx, "bob", "x", []byte("k"))
	login, err := h.Svc.LoginFinish(servertest.Ctx, id, "x", servertest.CorrectKE3([]byte("k"), bob.Record))
	if err != nil {
		t.Fatal(err)
	}
	return &idorRig{rig: r, cv: cv,
		callers: map[string]*servertest.User{"outsider": dave, "removed": erin, "viewer": carol, "editor": bob, "frozen": frank, "revoked": gina, "owner": alice},
		tokens: map[string]string{"none": "", "account token": login.AccountToken, "outsider": dave.Access, "removed": erin.Access,
			"viewer": carol.Access, "editor": bob.Access, "frozen": frank.Access, "revoked": ginaRevoked}}
}

// revokedSecondDevice certifies a second device for u, logs it in, then revokes it with a key
// succession, and returns the now-dead access token of that device.
func revokedSecondDevice(t *testing.T, h *servertest.Harness, u *servertest.User) string {
	second := servertest.RandomID(t)
	_, dsk2, _ := ed25519.GenerateKey(nil)
	if err := h.Svc.AppendDeviceLog(servertest.Ctx, h.Principal(u.Access), []vaultsync.DeviceEntry{u.AddDeviceEntry(t, second, dsk2)}); err != nil {
		t.Fatal(err)
	}
	token := h.DeviceLogin(u, second, dsk2)
	revoke := u.DeviceEntry(t, vaultsync.DeviceEntry{Op: vaultsync.OpRevokeDevice, Revoked: second})
	_, newAIK, _ := ed25519.GenerateKey(nil)
	succ := vaultsync.DeviceEntry{Account: u.Account, Seq: revoke.Seq + 1, Prev: u.Log.Hash(), Op: vaultsync.OpSuccession,
		NewSuite: vaultsync.Suite1, NewIdentityKey: newAIK.Public().(ed25519.PublicKey), NewEncryptionKey: bytes.Repeat([]byte{4}, 1216), SigAlg: vaultsync.SigEd25519}
	msg, _ := succ.SigningBytes()
	succ.Sig, succ.SuccessorSig = ed25519.Sign(u.AIK, msg), ed25519.Sign(newAIK, msg)
	if err := h.Svc.AppendDeviceLog(servertest.Ctx, h.Principal(u.Access), []vaultsync.DeviceEntry{revoke, succ}); err != nil {
		t.Fatal(err)
	}
	return token
}

func (ir *idorRig) commitBody(caller string) syncwire.Commit {
	u := ir.callers[caller]
	if u == nil {
		u = ir.callers["owner"]
	}
	c := ir.cv.Commit(u, ir.cv.Record(u, 7))
	return syncwire.Commit{ExpectSeq: ir.cv.State.Cursor().Seq, Head: syncwire.HeadFrom(&c.Head), Records: []syncwire.Envelope{syncwire.EnvelopeFrom(&c.Records[0])}}
}

func (ir *idorRig) membershipBody(caller string) syncwire.MembershipAppend {
	u := ir.callers[caller]
	if u == nil {
		u = ir.callers["owner"]
	}
	subject := servertest.RandomID(ir.t)
	ops := ir.cv.SignBatch(u, vaultsync.MemberOp{Kind: vaultsync.MemberAdd, Subject: subject, Role: vaultsync.RoleViewer,
		Verified: true, Epoch: ir.cv.M.Epoch(), Wraps: servertest.WrapFor(subject)})
	return syncwire.MembershipAppend{Ops: []syncwire.MemberOp{syncwire.MemberOpFrom(&ops[0])}}
}

// TestIDORMatrix probes every authenticated endpoint with every kind of caller. Each cell is a fixed
// expectation: what a vault endpoint must never do is answer an outsider differently from a vault
// that does not exist, or let a caller do more than its role.
func TestIDORMatrix(t *testing.T) {
	ir := newIDORRig(t)
	callers := []string{"none", "account token", "outsider", "removed", "viewer", "frozen", "revoked", "editor"}
	v := "/v1/vaults/" + ir.cv.ID.String()
	alice := ir.callers["owner"].Account.String()
	type endpoint struct {
		name, method, path string
		body               func(caller string) any
		want               map[string]int
	}
	reads := map[string]int{"none": 401, "account token": 401, "outsider": 404, "removed": 404, "viewer": 200, "frozen": 403, "revoked": 401, "editor": 200}
	endpoints := []endpoint{
		{"changes", "GET", v + "/changes", nil, reads},
		{"snapshot", "GET", v + "/snapshot", nil, reads},
		{"membership", "GET", v + "/membership", nil, reads},
		{"membership append", "POST", v + "/membership", func(c string) any { return ir.membershipBody(c) },
			map[string]int{"none": 401, "account token": 401, "outsider": 404, "removed": 404, "viewer": 403, "frozen": 403, "revoked": 401, "editor": 403}},
		{"device log of a co-member", "GET", "/v1/accounts/" + alice + "/devicelog", nil,
			map[string]int{"none": 401, "account token": 200, "outsider": 404, "removed": 404, "viewer": 200, "frozen": 403, "revoked": 401, "editor": 200}},
		{"lookup", "GET", "/v1/accounts/lookup?username=bob", nil,
			map[string]int{"none": 401, "account token": 401, "outsider": 200, "removed": 200, "viewer": 200, "frozen": 403, "revoked": 401, "editor": 200}},
		{"vault list", "GET", "/v1/vaults", nil,
			map[string]int{"none": 401, "account token": 401, "outsider": 200, "removed": 200, "viewer": 200, "frozen": 403, "revoked": 401, "editor": 200}},
		{"commit", "POST", v + "/commit", func(c string) any { return ir.commitBody(c) },
			map[string]int{"none": 401, "account token": 401, "outsider": 404, "removed": 404, "viewer": 403, "frozen": 403, "revoked": 401, "editor": 200}},
	}
	for _, ep := range endpoints {
		for _, caller := range callers {
			var body any
			if ep.body != nil {
				body = ep.body(caller)
			}
			status, raw := ir.do(ep.method, ep.path, ir.tokens[caller], body)
			if status != ep.want[caller] {
				t.Errorf("%s as %s: status %d, want %d (%s)", ep.name, caller, status, ep.want[caller], raw)
			}
			assertErrorBody(t, status, raw)
		}
	}
}

func TestExpiredTokenIsUnauthorized(t *testing.T) {
	r := newRig(t)
	u := r.h.Register("alice")
	r.h.Clock.Advance(15*60_000 + 1)
	status, body := r.do("GET", "/v1/vaults", u.Access, nil)
	if status != http.StatusUnauthorized {
		t.Fatalf("expired token: %d %s", status, body)
	}
}

func TestRateLimitPerAddress(t *testing.T) {
	h := servertest.NewHarness(t, 0)
	handler, _ := transport.New(transport.Config{IdentitySeed: identitySeed, RatePerSecond: 1, Burst: 2}, h.Svc)
	srv := httptest.NewServer(handler)
	defer srv.Close()
	r := &rig{t: t, h: h, srv: srv}
	var last int
	for i := 0; i < 3; i++ {
		last, _ = r.do("POST", "/v1/hello", "", syncwire.Hello{ClientProtocol: syncwire.Version{Major: 1}, Suite: 1})
	}
	if last != http.StatusTooManyRequests {
		t.Fatalf("third request within the burst of two: %d, want 429", last)
	}
}

func TestUnknownUsernameLoginLooksReal(t *testing.T) {
	r := newRig(t)
	r.h.Register("alice")
	statusA, a := r.do("POST", "/v1/login/init", "", syncwire.LoginInit{Username: "alice", KE1: []byte("k")})
	statusB, b := r.do("POST", "/v1/login/init", "", syncwire.LoginInit{Username: "nobody", KE1: []byte("k")})
	if statusA != statusB || len(a) != len(b) {
		t.Fatalf("known %d/%d bytes vs unknown %d/%d bytes; the reply must not reveal whether the account exists", statusA, len(a), statusB, len(b))
	}
}
