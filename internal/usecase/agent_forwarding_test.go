package usecase

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/agentforward"
)

// --- fixture ---

type agentTestKey struct {
	signer   gossh.Signer
	identity domain.SSHIdentity
}

func newAgentTestKey(t *testing.T, id string, allow bool) agentTestKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return agentTestKey{signer: signer, identity: domain.SSHIdentity{
		ID:              id,
		Comment:         "label of " + id,
		KeyType:         "ed25519",
		PublicKey:       strings.TrimSpace(string(gossh.MarshalAuthorizedKey(signer.PublicKey()))),
		Fingerprint:     gossh.FingerprintSHA256(signer.PublicKey()),
		AllowForwarding: allow,
	}}
}

type agentFakeConns struct {
	domain.ConnectionRepository
	mu   sync.Mutex
	conn domain.Connection
}

func (f *agentFakeConns) GetByID(_ context.Context, id string) (*domain.Connection, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if id != f.conn.ID {
		return nil, domain.ErrConnectionNotFound
	}
	c := domain.CloneConnection(f.conn)
	return &c, nil
}

func (f *agentFakeConns) edit(mutate func(*domain.Connection)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	mutate(&f.conn)
}

type agentFakeIdentities struct {
	domain.IdentityRepository
	mu   sync.Mutex
	byID map[string]domain.SSHIdentity
}

func (f *agentFakeIdentities) Get(_ context.Context, id string) (*domain.SSHIdentity, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i, ok := f.byID[id]
	if !ok {
		return nil, domain.ErrIdentityNotFound
	}
	return &i, nil
}

func (f *agentFakeIdentities) setAllow(id string, allow bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	i := f.byID[id]
	i.AllowForwarding = allow
	f.byID[id] = i
}

type agentFakeSigners struct {
	byID  map[string]gossh.Signer
	calls atomic.Int32
}

func (f *agentFakeSigners) SignerFor(_ context.Context, id string) (domain.Signer, error) {
	f.calls.Add(1)
	s, ok := f.byID[id]
	if !ok {
		return nil, domain.ErrIdentityNotFound
	}
	return s, nil
}

type agentFakeAudit struct {
	domain.AuditLogRepository
	mu      sync.Mutex
	entries []domain.AuditEntry
}

func (f *agentFakeAudit) Append(_ context.Context, e domain.AuditEntry) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.entries = append(f.entries, e)
	return nil
}

func (f *agentFakeAudit) lines() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.entries))
	for i, e := range f.entries {
		out[i] = e.Input
	}
	return out
}

type agentFakeKnownHosts struct {
	domain.KnownHostsRepository
	entries []domain.KnownHostEntry
}

func (f *agentFakeKnownHosts) List() ([]domain.KnownHostEntry, error) { return f.entries, nil }

// scriptedConfirm plays the user. It answers with decision and records every question.
type scriptedConfirm struct {
	mu        sync.Mutex
	decision  AgentDecision
	questions []AgentConfirmQuestion
	inFlight  atomic.Int32
	maxFlight atomic.Int32
	hold      chan struct{}
	entered   chan struct{}
}

func (s *scriptedConfirm) ask(ctx context.Context, q AgentConfirmQuestion) (AgentDecision, error) {
	n := s.inFlight.Add(1)
	defer s.inFlight.Add(-1)
	for {
		m := s.maxFlight.Load()
		if n <= m || s.maxFlight.CompareAndSwap(m, n) {
			break
		}
	}
	s.mu.Lock()
	s.questions = append(s.questions, q)
	d, hold, entered := s.decision, s.hold, s.entered
	s.mu.Unlock()
	if entered != nil {
		select {
		case entered <- struct{}{}:
		default:
		}
	}
	if hold != nil {
		select {
		case <-hold:
		case <-ctx.Done():
			return AgentDeny, ctx.Err()
		}
	}
	return d, nil
}

func (s *scriptedConfirm) asked() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.questions)
}

func (s *scriptedConfirm) last() AgentConfirmQuestion {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.questions[len(s.questions)-1]
}

type agentHarness struct {
	t        *testing.T
	keys     map[string]agentTestKey
	conns    *agentFakeConns
	idents   *agentFakeIdentities
	signers  *agentFakeSigners
	audit    *agentFakeAudit
	confirm  *scriptedConfirm
	svc      *AgentForwardingService
	clock    time.Time
	clockMu  sync.Mutex
	hostKey  gossh.Signer
	sessCtx  context.Context
	closeSes context.CancelFunc
}

func newAgentHarness(t *testing.T, confirm agentforward.ConfirmPolicy) *agentHarness {
	t.Helper()
	h := &agentHarness{
		t:       t,
		keys:    map[string]agentTestKey{},
		idents:  &agentFakeIdentities{byID: map[string]domain.SSHIdentity{}},
		signers: &agentFakeSigners{byID: map[string]gossh.Signer{}},
		audit:   &agentFakeAudit{},
		confirm: &scriptedConfirm{decision: AgentAllowOnce},
		clock:   time.Date(2026, 10, 10, 12, 0, 0, 0, time.UTC),
	}
	for _, k := range []agentTestKey{newAgentTestKey(t, "fwd", true), newAgentTestKey(t, "fwd2", true), newAgentTestKey(t, "noflag", false), newAgentTestKey(t, "unlisted", true)} {
		h.keys[k.identity.ID] = k
		h.idents.byID[k.identity.ID] = k.identity
		h.signers.byID[k.identity.ID] = k.signer
	}
	h.conns = &agentFakeConns{conn: domain.Connection{
		ID: "c1", Name: "bastion", Host: "bastion.corp", Port: 22,
		Users:         []domain.ConnectionUser{{ID: "u1", Username: "ops", Auth: domain.AuthMethodPassword, PassAuth: &domain.PasswordAuthConfig{VaultRef: "p"}}},
		DefaultUserID: "u1",
		AgentForwarding: &agentforward.Config{
			Enabled: true, IdentityIDs: []string{"fwd", "fwd2", "noflag"}, Confirm: confirm, WindowMinutes: 15,
		},
	}}
	_, hostPriv, _ := ed25519.GenerateKey(rand.Reader)
	h.hostKey, _ = gossh.NewSignerFromKey(hostPriv)
	h.svc = NewAgentForwardingService(AgentForwardingConfig{
		Connections: h.conns,
		Identities:  h.idents,
		KnownHosts: &agentFakeKnownHosts{entries: []domain.KnownHostEntry{
			{Host: "git.corp", Fingerprint: gossh.FingerprintSHA256(h.hostKey.PublicKey())},
		}},
		Signers: h.signers,
		Confirm: h.confirm.ask,
		Audit:   NewAgentAuditRecorder(h.audit),
		Now:     h.now,
	})
	h.sessCtx, h.closeSes = context.WithCancel(context.Background())
	t.Cleanup(h.closeSes)
	return h
}

func (h *agentHarness) now() time.Time {
	h.clockMu.Lock()
	defer h.clockMu.Unlock()
	return h.clock
}

func (h *agentHarness) advance(d time.Duration) {
	h.clockMu.Lock()
	defer h.clockMu.Unlock()
	h.clock = h.clock.Add(d)
}

func (h *agentHarness) policy() domain.AgentSignPolicy {
	h.t.Helper()
	return h.policyFor(h.sessCtx, "s1")
}

func (h *agentHarness) policyFor(ctx context.Context, sessionID string) domain.AgentSignPolicy {
	h.t.Helper()
	p, err := h.svc.PolicyFor(ctx, sessionID, "c1")
	if err != nil || p == nil {
		h.t.Fatalf("PolicyFor = %v, %v; an active connection must get a policy", p, err)
	}
	return p
}

var agentSID = []byte("agent-test-session-id-0123456789")

type loginFields struct {
	SessionID []byte
	Type      uint8
	User      string
	Service   string
	Method    string
	HasSig    bool
	Algorithm string
	Key       []byte
}

func loginData(user string, key gossh.PublicKey) []byte {
	return gossh.Marshal(loginFields{agentSID, 50, user, "ssh-connection", "publickey", true, key.Type(), key.Marshal()})
}

// bound returns a channel binding table in which agentSID is bound to the harness host key.
func (h *agentHarness) bound() *agentforward.Bindings {
	h.t.Helper()
	var b agentforward.Bindings
	if err := b.Add(agentforward.Binding{
		SessionID: agentSID, HostKey: h.hostKey.PublicKey().Marshal(),
		HostKeyFingerprint: gossh.FingerprintSHA256(h.hostKey.PublicKey()),
	}); err != nil {
		h.t.Fatal(err)
	}
	return &b
}

func (h *agentHarness) request(keyID string, b *agentforward.Bindings) domain.AgentSignRequest {
	pub := h.keys[keyID].signer.PublicKey()
	return domain.AgentSignRequest{KeyBlob: pub.Marshal(), Data: loginData("git", pub), Bindings: b}
}

// --- happy path ---

func TestAgentSignsAConfirmedLoginAndTheSignatureVerifies(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmEach)
	req := h.request("fwd", h.bound())
	sig, err := h.policy().Sign(context.Background(), req)
	if err != nil {
		t.Fatalf("a confirmed login for a forwarded key was refused: %v", err)
	}
	if err := h.keys["fwd"].signer.PublicKey().Verify(req.Data, sig); err != nil {
		t.Fatalf("the signature does not verify against the forwarded key: %v", err)
	}
	q := h.confirm.last()
	if q.RemoteUser != "git" || q.KeyFingerprint != h.keys["fwd"].identity.Fingerprint || q.ConnectionName != "bastion" {
		t.Errorf("the dialog must name the remote user, the key and the connection: %+v", q)
	}
	if !q.Destination.Verified || len(q.Destination.KnownAs) != 1 || q.Destination.KnownAs[0] != "git.corp" {
		t.Errorf("a bound destination must be shown verified and by its known_hosts name: %+v", q.Destination)
	}
}

func TestAgentListsOnlyKeysWithBothConsents(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmEach)
	keys, err := h.policy().PublicKeys(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]bool{}
	for _, k := range keys {
		for id, tk := range h.keys {
			if string(tk.signer.PublicKey().Marshal()) == string(k.Blob) {
				got[id] = true
			}
		}
	}
	if !got["fwd"] || !got["fwd2"] || got["noflag"] || got["unlisted"] || len(keys) != 2 {
		t.Fatalf("listed %v; only keys both listed by the connection and flagged on the key may be offered", got)
	}
}

// --- what the remote side may not obtain ---

func TestAgentRefusesAKeyTheConnectionDoesNotList(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmEach)
	_, err := h.policy().Sign(context.Background(), h.request("unlisted", h.bound()))
	h.expectRefusedSilently(err, domain.ErrAgentKeyNotForwardable)
}

func TestAgentRefusesAListedKeyWithoutItsOwnConsent(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmEach)
	_, err := h.policy().Sign(context.Background(), h.request("noflag", h.bound()))
	h.expectRefusedSilently(err, domain.ErrAgentKeyNotForwardable)
}

func TestAgentHonoursRevocationMidSession(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmEach)
	p := h.policy()
	if _, err := p.Sign(context.Background(), h.request("fwd", h.bound())); err != nil {
		t.Fatal(err)
	}
	h.idents.setAllow("fwd", false)
	if _, err := p.Sign(context.Background(), h.request("fwd", h.bound())); !errors.Is(err, domain.ErrAgentKeyNotForwardable) {
		t.Fatalf("err = %v; unticking the key must take effect at the next request", err)
	}
	h.conns.edit(func(c *domain.Connection) { c.AgentForwarding.Enabled = false })
	if _, err := p.Sign(context.Background(), h.request("fwd2", h.bound())); !errors.Is(err, domain.ErrAgentForwardingDisabled) {
		t.Fatalf("err = %v; switching forwarding off must take effect at the next request", err)
	}
	if keys, err := p.PublicKeys(context.Background()); err == nil || len(keys) != 0 {
		t.Errorf("a disabled connection must list nothing: %v, %v", keys, err)
	}
}

func TestAgentRefusesDataThatIsNotALogin(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmEach)
	pub := h.keys["fwd"].signer.PublicKey()
	cases := map[string][]byte{
		"git commit signature": append([]byte("SSHSIG"), gossh.Marshal(struct{ N, R, H, M string }{"git", "", "sha512", "x"})...),
		"arbitrary bytes":      []byte("transfer 1000000 to account 42"),
		"login with trailer":   append(loginData("git", pub), 0),
	}
	for name, data := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := h.policy().Sign(context.Background(), domain.AgentSignRequest{KeyBlob: pub.Marshal(), Data: data, Bindings: h.bound()})
			h.expectRefusedSilently(err, agentforward.ErrSignDataRejected)
		})
	}
}

func TestAgentRefusesALoginForAnotherKeyThanAskedFor(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmEach)
	req := h.request("fwd", h.bound())
	req.Data = loginData("git", h.keys["fwd2"].signer.PublicKey())
	_, err := h.policy().Sign(context.Background(), req)
	h.expectRefusedSilently(err, agentforward.ErrSignDataRejected)
}

func TestAgentRefusesAlgorithmFlagDisagreement(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmEach)
	req := h.request("fwd", h.bound())
	req.Flags = agentforward.FlagRSASHA256
	_, err := h.policy().Sign(context.Background(), req)
	h.expectRefusedSilently(err, agentforward.ErrAlgorithmRefused)
}

func TestAgentRefusesAHostboundLoginForAnotherHostThanItsBinding(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmEach)
	pub := h.keys["fwd"].signer.PublicKey()
	_, evilPriv, _ := ed25519.GenerateKey(rand.Reader)
	evil, _ := gossh.NewSignerFromKey(evilPriv)
	data := append(
		gossh.Marshal(loginFields{agentSID, 50, "git", "ssh-connection", "publickey-hostbound-v00@openssh.com", true, pub.Type(), pub.Marshal()}),
		gossh.Marshal(struct{ HostKey []byte }{evil.PublicKey().Marshal()})...,
	)
	_, err := h.policy().Sign(context.Background(), domain.AgentSignRequest{KeyBlob: pub.Marshal(), Data: data, Bindings: h.bound()})
	h.expectRefusedSilently(err, agentforward.ErrHostKeyMismatch)
}

func TestAgentDeniesWhenTheUserSaysNo(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmEach)
	h.confirm.decision = AgentDeny
	_, err := h.policy().Sign(context.Background(), h.request("fwd", h.bound()))
	if !errors.Is(err, domain.ErrAgentSignDenied) {
		t.Fatalf("err = %v, want ErrAgentSignDenied", err)
	}
	if h.signers.calls.Load() != 0 {
		t.Error("a denied request must not unlock the key")
	}
	h.expectAudit(AgentEventSignDenied, "reason=denied")
}

func TestAgentWithoutADialogDeniesEverything(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmEach)
	h.svc.cfg.Confirm = nil
	_, err := h.policy().Sign(context.Background(), h.request("fwd", h.bound()))
	if !errors.Is(err, domain.ErrAgentSignDenied) {
		t.Fatalf("err = %v; with nobody to ask, the answer must be no", err)
	}
}

func TestAgentRefusesWhenTheVaultKeyIsNotTheMatchedOne(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmEach)
	h.signers.byID["fwd"] = h.keys["fwd2"].signer
	_, err := h.policy().Sign(context.Background(), h.request("fwd", h.bound()))
	if !errors.Is(err, domain.ErrAgentKeyNotForwardable) {
		t.Fatalf("err = %v; a signer that is not the matched key must not sign", err)
	}
}

// --- windows ---

func TestAgentWindowCoversOnlyTheSameKeyAndVerifiedDestination(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmSessionWindow)
	h.confirm.decision = AgentAllowWindow
	p := h.policy()
	if _, err := p.Sign(context.Background(), h.request("fwd", h.bound())); err != nil {
		t.Fatal(err)
	}
	if q := h.confirm.last(); q.WindowMinutes != 15 {
		t.Fatalf("a verified destination on a session-window connection must offer the window, got %d", q.WindowMinutes)
	}
	if _, err := p.Sign(context.Background(), h.request("fwd", h.bound())); err != nil || h.confirm.asked() != 1 {
		t.Fatalf("within the window the same key and host must sign without asking (asked %d, %v)", h.confirm.asked(), err)
	}
	if _, err := p.Sign(context.Background(), h.request("fwd2", h.bound())); err != nil || h.confirm.asked() != 2 {
		t.Fatalf("another key must ask again (asked %d)", h.confirm.asked())
	}
	h.confirm.decision = AgentAllowOnce
	if _, err := p.Sign(context.Background(), h.request("fwd", nil)); err != nil || h.confirm.asked() != 3 {
		t.Fatalf("an unverified destination must ask again (asked %d, %v)", h.confirm.asked(), err)
	}
	h.confirm.decision = AgentAllowWindow
	other := h.policyFor(h.sessCtx, "s2")
	if _, err := other.Sign(context.Background(), h.request("fwd", h.bound())); err != nil || h.confirm.asked() != 4 {
		t.Fatalf("another session must ask again (asked %d)", h.confirm.asked())
	}
	h.advance(15 * time.Minute)
	if _, err := p.Sign(context.Background(), h.request("fwd", h.bound())); err != nil || h.confirm.asked() != 5 {
		t.Fatalf("an expired window must ask again (asked %d)", h.confirm.asked())
	}
}

func TestAgentNeverOffersAWindowForAnUnverifiedDestination(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmSessionWindow)
	h.confirm.decision = AgentAllowWindow
	_, err := h.policy().Sign(context.Background(), h.request("fwd", nil))
	if q := h.confirm.last(); q.WindowMinutes != 0 || q.Destination.Verified {
		t.Fatalf("an unbound destination must be shown unverified with no window: %+v", q)
	}
	if !errors.Is(err, domain.ErrAgentSignDenied) {
		t.Fatalf("err = %v; a window answer to a question that offered none must be a denial", err)
	}
}

func TestAgentConfirmEachOffersNoWindow(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmEach)
	if _, err := h.policy().Sign(context.Background(), h.request("fwd", h.bound())); err != nil {
		t.Fatal(err)
	}
	if q := h.confirm.last(); q.WindowMinutes != 0 {
		t.Fatalf("a confirm-each connection must not offer a window, got %d", q.WindowMinutes)
	}
}

// --- lifetime and bounds ---

func TestAgentRefusesEverythingOnceTheSessionEnds(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmSessionWindow)
	h.confirm.decision = AgentAllowWindow
	p := h.policy()
	if _, err := p.Sign(context.Background(), h.request("fwd", h.bound())); err != nil {
		t.Fatal(err)
	}
	h.closeSes()
	if _, err := p.Sign(context.Background(), h.request("fwd", h.bound())); !errors.Is(err, domain.ErrAgentSessionEnded) {
		t.Fatalf("err = %v; a closed session's agent must refuse even inside its window", err)
	}
	if _, err := p.PublicKeys(context.Background()); !errors.Is(err, domain.ErrAgentSessionEnded) {
		t.Fatalf("err = %v; a closed session's agent must list nothing", err)
	}
}

func TestAgentReconnectEndsThePreviousPolicyAndItsWindow(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmSessionWindow)
	h.confirm.decision = AgentAllowWindow
	old := h.policy()
	if _, err := old.Sign(context.Background(), h.request("fwd", h.bound())); err != nil {
		t.Fatal(err)
	}
	fresh := h.policy()
	if _, err := old.Sign(context.Background(), h.request("fwd", h.bound())); !errors.Is(err, domain.ErrAgentSessionEnded) {
		t.Fatalf("err = %v; the previous connection's agent must be dead", err)
	}
	if _, err := fresh.Sign(context.Background(), h.request("fwd", h.bound())); err != nil || h.confirm.asked() != 2 {
		t.Fatalf("a window must not survive a reconnect (asked %d, %v)", h.confirm.asked(), err)
	}
}

// The old connection's context can end after its replacement registered - a dropped link is
// noticed late. Its cleanup must take only what was its own.
func TestAgentLateTeardownOfTheOldConnectionSparesTheNewOne(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmEach)
	old := h.policy().(*agentSessionPolicy)
	fresh := h.policy()
	// What the old context's AfterFunc does, run synchronously so the test does not race it.
	h.svc.forget(old)
	if _, err := fresh.Sign(context.Background(), h.request("fwd", h.bound())); err != nil {
		t.Fatalf("err = %v; the replaced connection's teardown ended its successor's agent", err)
	}
}

func TestAgentLimitsTheRateOfRequests(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmEach)
	p := h.policy()
	for i := range agentforward.SignRequestsPerWindow {
		if _, err := p.Sign(context.Background(), h.request("fwd", h.bound())); err != nil {
			t.Fatalf("request %d: %v", i+1, err)
		}
	}
	_, err := p.Sign(context.Background(), h.request("fwd", h.bound()))
	if !errors.Is(err, agentforward.ErrRateLimited) {
		t.Fatalf("err = %v; request %d inside the window must be refused", err, agentforward.SignRequestsPerWindow+1)
	}
	if h.confirm.asked() != agentforward.SignRequestsPerWindow {
		t.Error("a rate-limited request must not reach the user")
	}
}

func TestAgentAsksOneQuestionAtATimePerSession(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmEach)
	h.confirm.hold = make(chan struct{})
	h.confirm.entered = make(chan struct{}, 1)
	p := h.policy()
	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			_, _ = p.Sign(context.Background(), h.request("fwd", h.bound()))
		})
	}
	// One dialog is open and held; the other three are queued behind it or still arriving. Holding
	// the first is what would expose a second dialog opening alongside it.
	<-h.confirm.entered
	close(h.confirm.hold)
	wg.Wait()
	if got := h.confirm.maxFlight.Load(); got != 1 {
		t.Fatalf("%d dialogs were open at once for one session; a server must not stack them", got)
	}
}

func TestAgentPolicyIsOnlyForActiveSSHConnections(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmEach)
	h.conns.edit(func(c *domain.Connection) { c.AgentForwarding.Enabled = false })
	if p, err := h.svc.PolicyFor(h.sessCtx, "s1", "c1"); p != nil || err != nil {
		t.Fatalf("a disabled connection got a policy: %v, %v", p, err)
	}
	h.conns.edit(func(c *domain.Connection) { c.AgentForwarding.Enabled = true; c.Protocol = "rdp" })
	if p, err := h.svc.PolicyFor(h.sessCtx, "s1", "c1"); p != nil || err != nil {
		t.Fatalf("a plugin protocol got a policy: %v, %v", p, err)
	}
	if _, err := h.svc.PolicyFor(h.sessCtx, "s1", "missing"); !errors.Is(err, domain.ErrConnectionNotFound) {
		t.Fatalf("err = %v; an unknown connection is an error, not silent forwarding", err)
	}
}

// --- audit ---

func TestAgentAuditCarriesFingerprintsAndNeverTheSignedData(t *testing.T) {
	h := newAgentHarness(t, agentforward.ConfirmEach)
	p := h.policy()
	req := h.request("fwd", h.bound())
	sig, err := p.Sign(context.Background(), req)
	if err != nil {
		t.Fatal(err)
	}
	p.Refuse(context.Background(), "add-identity")
	h.expectAudit(AgentEventForwardStarted, "")
	allowed := h.expectAudit(AgentEventSignAllowed, "key="+h.keys["fwd"].identity.Fingerprint)
	for _, want := range []string{"remote-user=git", "destination=" + gossh.FingerprintSHA256(h.hostKey.PublicKey()), "verified"} {
		if !strings.Contains(allowed, want) {
			t.Errorf("allowed entry %q lacks %q", allowed, want)
		}
	}
	h.expectAudit(AgentEventOpRefused, "reason=add-identity")
	for _, line := range h.audit.lines() {
		for _, secret := range []string{base64.StdEncoding.EncodeToString(req.Data), base64.StdEncoding.EncodeToString(sig.Blob), string(agentSID)} {
			if strings.Contains(line, secret) {
				t.Fatalf("audit line %q carries signed data or a signature", line)
			}
		}
	}
}

func (h *agentHarness) expectRefusedSilently(err, want error) {
	h.t.Helper()
	if !errors.Is(err, want) {
		h.t.Fatalf("err = %v, want %v", err, want)
	}
	if h.confirm.asked() != 0 {
		h.t.Error("a request the policy refuses on its own must never reach the user")
	}
	if h.signers.calls.Load() != 0 {
		h.t.Error("a refused request must not unlock any key")
	}
	h.expectAudit(AgentEventSignDenied, "reason=")
}

func (h *agentHarness) expectAudit(event, contains string) string {
	h.t.Helper()
	for _, line := range h.audit.lines() {
		if strings.HasPrefix(line, event+" ") || line == event {
			if strings.Contains(line, contains) {
				return line
			}
		}
	}
	h.t.Fatalf("no %s audit entry containing %q in %q", event, contains, h.audit.lines())
	return ""
}
