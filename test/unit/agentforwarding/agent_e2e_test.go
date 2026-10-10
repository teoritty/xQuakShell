// Package agentforwarding drives agent forwarding end to end with the application composed as the
// composition root composes it: a real vault on disk, the real key codec and key manager, the real
// dialer, known_hosts check and PTY bridge, and the real session manager.
//
// Two in-process SSH servers stand in for the network. A is the server the user opens a shell on.
// The test then plays a process on A - `ssh B` or `git pull` - which reaches the forwarded agent
// through the auth-agent channel A opens back to the client, binds its session to B the way OpenSSH
// 8.9+ does, and authenticates to B with a key that never left the vault. Only the confirmation
// dialog is played by the test, through the same Resolve the RPC calls.
package agentforwarding

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/agentforward"
	"xquakshell/internal/infra/keys"
	"xquakshell/internal/infra/persistence"
	infrassh "xquakshell/internal/infra/ssh"
	"xquakshell/internal/pkg/conlimit"
	"xquakshell/internal/usecase"
)

const waitTimeout = 15 * time.Second

func newHostKey(t *testing.T) gossh.Signer {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	s, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func listen(t *testing.T) net.Listener {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln
}

// --- server A: the shell the user opens ---

type shellServer struct {
	addr      *net.TCPAddr
	hostKey   gossh.Signer
	conns     chan *gossh.ServerConn
	forwarded chan struct{}
}

func startShellServer(t *testing.T) *shellServer {
	t.Helper()
	s := &shellServer{hostKey: newHostKey(t), conns: make(chan *gossh.ServerConn, 4), forwarded: make(chan struct{}, 4)}
	cfg := &gossh.ServerConfig{
		PasswordCallback: func(gossh.ConnMetadata, []byte) (*gossh.Permissions, error) { return &gossh.Permissions{}, nil },
	}
	cfg.AddHostKey(s.hostKey)
	ln := listen(t)
	s.addr = ln.Addr().(*net.TCPAddr)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go s.serve(c, cfg)
		}
	}()
	return s
}

func (s *shellServer) serve(c net.Conn, cfg *gossh.ServerConfig) {
	sc, chans, reqs, err := gossh.NewServerConn(c, cfg)
	if err != nil {
		_ = c.Close()
		return
	}
	s.conns <- sc
	go gossh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			_ = nc.Reject(gossh.UnknownChannelType, "")
			continue
		}
		ch, chReqs, err := nc.Accept()
		if err != nil {
			continue
		}
		go s.session(ch, chReqs)
	}
}

func (s *shellServer) session(ch gossh.Channel, reqs <-chan *gossh.Request) {
	for r := range reqs {
		ok := r.Type == "pty-req" || r.Type == "shell" || r.Type == "auth-agent-req@openssh.com" || r.Type == "window-change"
		if r.Type == "auth-agent-req@openssh.com" {
			s.forwarded <- struct{}{}
		}
		if r.WantReply {
			_ = r.Reply(ok, nil)
		}
		if r.Type == "shell" {
			_, _ = ch.Write([]byte("$ "))
		}
	}
}

// awaitForwarded waits for the client's auth-agent-req and returns the connection it came on.
func (s *shellServer) awaitForwarded(t *testing.T) *gossh.ServerConn {
	t.Helper()
	select {
	case <-s.forwarded:
	case <-time.After(waitTimeout):
		t.Fatal("the shell never asked for agent forwarding")
	}
	select {
	case sc := <-s.conns:
		return sc
	case <-time.After(waitTimeout):
		t.Fatal("no server connection recorded")
		return nil
	}
}

// --- server B: the next host, reachable only with the forwarded key ---

type keyServer struct {
	addr     *net.TCPAddr
	hostKey  gossh.Signer
	mu       sync.Mutex
	allowed  gossh.PublicKey
	accepted int
}

func startKeyServer(t *testing.T) *keyServer {
	t.Helper()
	s := &keyServer{hostKey: newHostKey(t)}
	cfg := &gossh.ServerConfig{
		PublicKeyCallback: func(_ gossh.ConnMetadata, key gossh.PublicKey) (*gossh.Permissions, error) {
			s.mu.Lock()
			defer s.mu.Unlock()
			if s.allowed == nil || !bytes.Equal(key.Marshal(), s.allowed.Marshal()) {
				return nil, errors.New("key not authorized")
			}
			return &gossh.Permissions{}, nil
		},
	}
	cfg.AddHostKey(s.hostKey)
	ln := listen(t)
	s.addr = ln.Addr().(*net.TCPAddr)
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				sc, chans, reqs, err := gossh.NewServerConn(c, cfg)
				if err != nil {
					_ = c.Close()
					return
				}
				s.mu.Lock()
				s.accepted++
				s.mu.Unlock()
				go gossh.DiscardRequests(reqs)
				go func() {
					for nc := range chans {
						_ = nc.Reject(gossh.Prohibited, "")
					}
				}()
				_ = sc.Wait()
			}()
		}
	}()
	return s
}

func (s *keyServer) authorize(key gossh.PublicKey) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.allowed = key
}

// --- the process on A ---

// bindingSigner is what OpenSSH's ssh does before asking the agent to sign a login: it sends
// session-bind with the destination's host key and that host's signature over the session id. In
// a real exchange the signature comes from the key exchange; the test owns B's host key, so it
// produces the same bytes B produced.
type bindingSigner struct {
	agent    agent.ExtendedAgent
	inner    gossh.Signer
	hostKey  gossh.Signer
	noBind   bool
	bindErrs []error
}

func (b *bindingSigner) PublicKey() gossh.PublicKey { return b.inner.PublicKey() }

func (b *bindingSigner) Sign(r io.Reader, data []byte) (*gossh.Signature, error) {
	if !b.noBind {
		var sid []byte
		if len(data) >= 4 {
			n := int(data[0])<<24 | int(data[1])<<16 | int(data[2])<<8 | int(data[3])
			if n <= len(data)-4 {
				sid = data[4 : 4+n]
			}
		}
		hostSig, err := b.hostKey.Sign(rand.Reader, sid)
		if err != nil {
			return nil, err
		}
		contents := gossh.Marshal(struct {
			HostKey, SessionID, Signature []byte
			Forwarding                    bool
		}{b.hostKey.PublicKey().Marshal(), sid, gossh.Marshal(hostSig), false})
		if _, err := b.agent.Extension("session-bind@openssh.com", contents); err != nil {
			b.bindErrs = append(b.bindErrs, err)
		}
	}
	return b.inner.Sign(r, data)
}

// remote is a process on A holding the forwarded agent.
type remote struct {
	t     *testing.T
	agent agent.ExtendedAgent
	ch    gossh.Channel
}

func openAgent(t *testing.T, sc *gossh.ServerConn) (*remote, error) {
	t.Helper()
	ch, reqs, err := sc.OpenChannel("auth-agent@openssh.com", nil)
	if err != nil {
		return nil, err
	}
	go gossh.DiscardRequests(reqs)
	t.Cleanup(func() { _ = ch.Close() })
	return &remote{t: t, agent: agent.NewClient(ch), ch: ch}, nil
}

// loginTo runs `ssh B` from A: it authenticates to B with whatever the agent will sign.
func (r *remote) loginTo(b *keyServer, bind bool) error {
	r.t.Helper()
	signers, err := r.agent.Signers()
	if err != nil {
		return fmt.Errorf("list agent keys: %w", err)
	}
	wrapped := make([]gossh.Signer, 0, len(signers))
	for _, s := range signers {
		wrapped = append(wrapped, &bindingSigner{agent: r.agent, inner: s, hostKey: b.hostKey, noBind: !bind})
	}
	client, err := gossh.Dial("tcp", b.addr.String(), &gossh.ClientConfig{
		User:            "git",
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(wrapped...)},
		HostKeyCallback: gossh.FixedHostKey(b.hostKey.PublicKey()),
		Timeout:         waitTimeout,
	})
	if err != nil {
		return err
	}
	return client.Close()
}

// --- the dialog ---

type dialog struct {
	prompts *usecase.AgentConfirmPrompts
	shown   chan usecase.AgentConfirmPrompt
	answer  usecase.AgentDecision
	mu      sync.Mutex
	asked   []usecase.AgentConfirmPrompt
}

func (d *dialog) ShowAgentConfirm(p usecase.AgentConfirmPrompt) {
	d.mu.Lock()
	d.asked = append(d.asked, p)
	answer := d.answer
	d.mu.Unlock()
	// The user clicks as soon as the dialog appears, through the same Resolve the RPC calls.
	go func() { _ = d.prompts.Resolve(p.SessionID, p.RequestID, answer) }()
}

func (d *dialog) DismissAgentConfirm(string) {}

func (d *dialog) setAnswer(a usecase.AgentDecision) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.answer = a
}

func (d *dialog) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.asked)
}

func (d *dialog) last() usecase.AgentConfirmPrompt {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.asked[len(d.asked)-1]
}

// --- audit ---

type memoryAudit struct {
	domain.AuditLogRepository
	mu    sync.Mutex
	lines []string
}

func (m *memoryAudit) Append(_ context.Context, e domain.AuditEntry) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lines = append(m.lines, e.Input)
	return nil
}

func (m *memoryAudit) has(prefix, contains string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, l := range m.lines {
		if strings.HasPrefix(l, prefix) && strings.Contains(l, contains) {
			return true
		}
	}
	return false
}

type noSFTP struct{}

func (noSFTP) New(domain.SSHClient, int) (domain.RemoteFS, error) {
	return nil, errors.New("no sftp in this test")
}

// --- the application ---

type fixture struct {
	t         *testing.T
	sessions  *usecase.SessionManager
	keyMgr    *usecase.KeyManagerService
	connRepo  domain.ConnectionRepository
	dialog    *dialog
	audit     *memoryAudit
	a         *shellServer
	b         *keyServer
	states    chan domain.ConnectionSession
	forwarded *domain.SSHIdentity
	connID    string
}

func newFixture(t *testing.T, confirm agentforward.ConfirmPolicy) *fixture {
	t.Helper()
	ctx := context.Background()
	vaultRepo := persistence.NewVaultRepo(t.TempDir())
	if err := vaultRepo.Create(ctx, "agent forwarding e2e master password"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(vaultRepo.Lock)
	codec := keys.NewCodec()
	identRepo := persistence.NewIdentityRepo(vaultRepo, codec, keys.NewDataKey)
	connRepo := persistence.NewConnectionRepo(vaultRepo)
	knownHosts := persistence.NewKnownHostsRepo(vaultRepo)
	passwords := persistence.NewPasswordRepo(vaultRepo)
	cache := infrassh.NewPassphraseCache()
	keyMgr := usecase.NewKeyManagerService(usecase.KeyManagerConfig{
		Identities: identRepo, Vault: vaultRepo, Codec: codec, Cache: cache, NewDataKey: keys.NewDataKey,
	})

	f := &fixture{
		t: t, keyMgr: keyMgr, connRepo: connRepo, audit: &memoryAudit{},
		a: startShellServer(t), b: startKeyServer(t), states: make(chan domain.ConnectionSession, 64), connID: "conn-fwd",
	}
	f.dialog = &dialog{prompts: &usecase.AgentConfirmPrompts{}, answer: usecase.AgentAllowOnce}

	forwarded, err := keyMgr.Generate(ctx, domain.GeneratedKeySpec{Algorithm: domain.AlgorithmEd25519, Comment: "deploy key"}, "",
		usecase.KeyOptions{AllowForwarding: true})
	if err != nil {
		t.Fatal(err)
	}
	f.forwarded = forwarded
	pub, _, _, _, err := gossh.ParseAuthorizedKey([]byte(forwarded.PublicKey))
	if err != nil {
		t.Fatal(err)
	}
	f.b.authorize(pub)

	for host, key := range map[string]gossh.PublicKey{
		fmt.Sprintf("[127.0.0.1]:%d", f.a.addr.Port): f.a.hostKey.PublicKey(),
		"git.corp": f.b.hostKey.PublicKey(),
	} {
		if err := knownHosts.Add(ctx, host, key); err != nil {
			t.Fatal(err)
		}
	}
	pwRef, err := passwords.Import(ctx, []byte("bastion password"), "bastion")
	if err != nil {
		t.Fatal(err)
	}
	if err := connRepo.Save(ctx, &domain.Connection{
		ID: f.connID, Name: "bastion", Host: "127.0.0.1", Port: f.a.addr.Port, Protocol: domain.ProtocolSSH,
		Users:         []domain.ConnectionUser{{ID: "u1", Username: "ops", Auth: domain.AuthMethodPassword, PassAuth: &domain.PasswordAuthConfig{VaultRef: pwRef}}},
		DefaultUserID: "u1",
		AgentForwarding: &agentforward.Config{
			Enabled: true, IdentityIDs: []string{forwarded.ID}, Confirm: confirm, WindowMinutes: 15,
		},
	}); err != nil {
		t.Fatal(err)
	}

	f.sessions = usecase.NewSessionManager(usecase.SessionManagerConfig{
		ConnRepo: connRepo, VaultRepo: vaultRepo, IdentRepo: identRepo, PasswordRepo: passwords,
		KnownHosts: knownHosts, SSHFactory: infrassh.NewDialer(), PassphraseCache: cache,
		HostKeyCallbackBuilder: infrassh.NewHostKeyCallbackBuilder(), JumpTransportBuilder: infrassh.NewJumpTransportBuilder(),
		Keys: keyMgr, PTYBridgeFactory: infrassh.NewPTYBridgeFactory(), SFTPClientFactory: noSFTP{},
		ForwardConnLimiterFactory: func() domain.ConcurrencyLimiter { return conlimit.New(4) },
		OnStateChange:             func(s domain.ConnectionSession) { f.states <- s },
		// The same shape AppAPI.onAgentConfirm has, with the dialog in place of the Wails emitter.
		AgentConfirm: func(ctx context.Context, q usecase.AgentConfirmQuestion) (usecase.AgentDecision, error) {
			return f.dialog.prompts.Ask(ctx, q, f.dialog)
		},
		AgentAudit: f.audit,
	})
	t.Cleanup(f.sessions.CloseAll)
	return f
}

// openShell opens a session and starts its shell, as the frontend does, and returns the session id
// together with server A's side of the connection.
func (f *fixture) openShell() (string, *gossh.ServerConn) {
	f.t.Helper()
	sid := f.openReady()
	out, _, err := f.sessions.InitSessionIO(context.Background(), sid)
	if err != nil || out == nil {
		f.t.Fatalf("InitSessionIO = %v, %v; the shell did not start", out, err)
	}
	go func() {
		for range out {
		}
	}()
	return sid, f.a.awaitForwarded(f.t)
}

// openReady opens a session and waits until its SSH connection is established.
func (f *fixture) openReady() string {
	f.t.Helper()
	sid, err := f.sessions.OpenSession(context.Background(), f.connID)
	if err != nil {
		f.t.Fatal(err)
	}
	deadline := time.After(waitTimeout)
	for {
		select {
		case s := <-f.states:
			if s.SessionID == sid && s.State == domain.SessionError {
				f.t.Fatalf("session failed: %s", s.ErrorMessage)
			}
			if s.SessionID == sid && s.State == domain.SessionReady {
				return sid
			}
		case <-deadline:
			f.t.Fatal("session never became ready")
		}
	}
}

func (f *fixture) setAllowForwarding(allow bool) {
	f.t.Helper()
	if err := f.keyMgr.SetPolicy(context.Background(), f.forwarded.ID, usecase.KeyOptions{
		Cache: domain.CacheUntilLock, AllowForwarding: allow,
	}); err != nil {
		f.t.Fatal(err)
	}
}

// --- scenarios ---

func TestNextHopLoginThroughTheForwardedAgent(t *testing.T) {
	f := newFixture(t, agentforward.ConfirmEach)
	_, a := f.openShell()
	r, err := openAgent(t, a)
	if err != nil {
		t.Fatalf("server A could not reach the forwarded agent: %v", err)
	}
	if err := r.loginTo(f.b, true); err != nil {
		t.Fatalf("`ssh B` from A failed with an approved signature: %v", err)
	}
	q := f.dialog.last()
	if !q.Destination.Verified || len(q.Destination.KnownAs) != 1 || q.Destination.KnownAs[0] != "git.corp" {
		t.Fatalf("the dialog must name B as a verified known host: %+v", q.Destination)
	}
	if q.RemoteUser != "git" || q.KeyFingerprint != f.forwarded.Fingerprint || q.ConnectionName != "bastion" {
		t.Fatalf("the dialog must say who asks for which key as whom: %+v", q)
	}
	if !f.audit.has(usecase.AgentEventSignAllowed, "destination="+gossh.FingerprintSHA256(f.b.hostKey.PublicKey())) {
		t.Fatalf("the signature is not in the audit log: %q", f.audit.lines)
	}
}

func TestDeniedSignatureKeepsTheNextHopClosed(t *testing.T) {
	f := newFixture(t, agentforward.ConfirmEach)
	f.dialog.setAnswer(usecase.AgentDeny)
	_, a := f.openShell()
	r, err := openAgent(t, a)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.loginTo(f.b, true); err == nil {
		t.Fatal("B let A in although the user denied the signature")
	}
	if !f.audit.has(usecase.AgentEventSignDenied, "reason=denied") {
		t.Fatalf("the denial is not in the audit log: %q", f.audit.lines)
	}
}

func TestAKeyWithoutItsOwnConsentIsNeverOffered(t *testing.T) {
	f := newFixture(t, agentforward.ConfirmEach)
	f.setAllowForwarding(false)
	_, a := f.openShell()
	r, err := openAgent(t, a)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := r.agent.List()
	if err != nil || len(keys) != 0 {
		t.Fatalf("listed %d keys (%v); a key not consenting to forwarding must be invisible", len(keys), err)
	}
	if err := r.loginTo(f.b, true); err == nil {
		t.Fatal("B let A in with a key that was never forwardable")
	}
	if f.dialog.count() != 0 {
		t.Fatal("the user was asked about a key the policy must refuse by itself")
	}
}

func TestRevokingConsentMidSessionStopsTheNextSignature(t *testing.T) {
	f := newFixture(t, agentforward.ConfirmEach)
	_, a := f.openShell()
	r, err := openAgent(t, a)
	if err != nil {
		t.Fatal(err)
	}
	if err := r.loginTo(f.b, true); err != nil {
		t.Fatal(err)
	}
	f.setAllowForwarding(false)
	if err := r.loginTo(f.b, true); err == nil {
		t.Fatal("a key unticked mid-session still signed")
	}
}

func TestAWindowCoversRepeatedLoginsToTheSameVerifiedHost(t *testing.T) {
	f := newFixture(t, agentforward.ConfirmSessionWindow)
	f.dialog.setAnswer(usecase.AgentAllowWindow)
	_, a := f.openShell()
	r, err := openAgent(t, a)
	if err != nil {
		t.Fatal(err)
	}
	for i := range 3 {
		if err := r.loginTo(f.b, true); err != nil {
			t.Fatalf("login %d inside the window failed: %v", i+1, err)
		}
	}
	if n := f.dialog.count(); n != 1 {
		t.Fatalf("the user was asked %d times; one window answer covers the same key and host", n)
	}
}

func TestAnUnboundDestinationIsAskedEveryTimeAndOfferedNoWindow(t *testing.T) {
	f := newFixture(t, agentforward.ConfirmSessionWindow)
	_, a := f.openShell()
	r, err := openAgent(t, a)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := r.loginTo(f.b, false); err != nil {
			t.Fatal(err)
		}
	}
	q := f.dialog.last()
	if q.Destination.Verified || q.WindowMinutes != 0 || f.dialog.count() != 2 {
		t.Fatalf("an unbound login must be unverified, offer no window and ask each time: %+v (asked %d)", q, f.dialog.count())
	}
}

func TestAHostileServerCannotUseTheAgentAsASigningOracle(t *testing.T) {
	f := newFixture(t, agentforward.ConfirmEach)
	_, a := f.openShell()
	r, err := openAgent(t, a)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := r.agent.List()
	if err != nil || len(keys) != 1 {
		t.Fatalf("List = %v, %v", keys, err)
	}
	if _, err := r.agent.Sign(keys[0], []byte("SSHSIG\x00\x00\x00\x03git")); err == nil {
		t.Fatal("the agent signed a git commit signature")
	}
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	if err := r.agent.Add(agent.AddedKey{PrivateKey: priv}); err == nil {
		t.Fatal("the remote side added a key to the agent")
	}
	if err := r.agent.RemoveAll(); err == nil {
		t.Fatal("the remote side removed the agent's keys")
	}
	if f.dialog.count() != 0 {
		t.Fatal("hostile requests must be refused without bothering the user")
	}
	if !f.audit.has(usecase.AgentEventOpRefused, "reason=add-identity") || !f.audit.has(usecase.AgentEventSignDenied, "reason=not-a-login") {
		t.Fatalf("hostile requests must be audited: %q", f.audit.lines)
	}
}

func TestClosingTheSessionEndsTheAgent(t *testing.T) {
	f := newFixture(t, agentforward.ConfirmEach)
	sid, a := f.openShell()
	r, err := openAgent(t, a)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := r.agent.List(); err != nil {
		t.Fatal(err)
	}
	if err := f.sessions.CloseSession(sid); err != nil {
		t.Fatal(err)
	}
	if _, err := r.agent.List(); err == nil {
		t.Fatal("the agent answered after its session was closed")
	}
}

func TestAConnectionWithoutForwardingOffersNoAgent(t *testing.T) {
	f := newFixture(t, agentforward.ConfirmEach)
	conn, err := f.connRepo.GetByID(context.Background(), f.connID)
	if err != nil {
		t.Fatal(err)
	}
	conn.AgentForwarding.Enabled = false
	if err := f.connRepo.Save(context.Background(), conn); err != nil {
		t.Fatal(err)
	}
	sid := f.openReady()
	if _, _, err := f.sessions.InitSessionIO(context.Background(), sid); err != nil {
		t.Fatal(err)
	}
	select {
	case <-f.a.forwarded:
		t.Fatal("a connection with forwarding off asked the server to forward an agent")
	case sc := <-f.a.conns:
		if _, err := openAgent(t, sc); err == nil {
			t.Fatal("the server reached an agent the user never offered")
		}
	case <-time.After(waitTimeout):
		t.Fatal("no connection reached server A")
	}
}
