//go:build e2e_docker

// Package agentforwarding checks agent forwarding against real OpenSSH: the scenarios the feature
// exists for, run in a shell on a bastion container exactly as a user would type them - `ssh` to
// the next host, `git clone` of a private repository, `scp` and `rsync` between the two servers.
//
// The application side is the real stack (vault, key manager, dialer, PTY bridge, session
// manager). The remote side is OpenSSH 9.7, whose client sends session-bind and signs
// publickey-hostbound requests; the in-process e2e in test/unit/agentforwarding emulates those,
// and this test is where the emulation is checked against the implementation.
//
// Run with `make e2e-agent`. It needs a running Docker daemon and is not part of `go test ./...`.
package agentforwarding

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/pem"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/agentforward"
	"xquakshell/internal/infra/keys"
	"xquakshell/internal/infra/persistence"
	infrassh "xquakshell/internal/infra/ssh"
	"xquakshell/internal/pkg/conlimit"
	"xquakshell/internal/usecase"
)

const (
	startTimeout   = 5 * time.Minute
	commandTimeout = 60 * time.Second
)

// --- docker ---

type project struct {
	t    *testing.T
	name string
	dir  string
	env  []string
}

func (p *project) compose(args ...string) (string, error) {
	cmd := exec.Command("docker", append([]string{"compose", "-p", p.name, "-f", filepath.Join(p.dir, "compose.yml")}, args...)...) // #nosec G204 -- fixed binary, arguments built by this test
	cmd.Env = append(os.Environ(), p.env...)
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	err := cmd.Run()
	return out.String(), err
}

func startProject(t *testing.T, seed string) (*project, string) {
	t.Helper()
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker is not installed")
	}
	dir, err := filepath.Abs("docker")
	if err != nil {
		t.Fatal(err)
	}
	p := &project{t: t, name: fmt.Sprintf("xqagent%d", time.Now().UnixNano()%1e9), dir: dir, env: []string{"SEED_DIR=" + seed}}
	t.Cleanup(func() {
		if out, err := p.compose("down", "-v", "--remove-orphans", "-t", "1"); err != nil {
			t.Logf("compose down: %v\n%s", err, out)
		}
	})
	if out, err := p.compose("up", "-d", "--build"); err != nil {
		t.Fatalf("compose up: %v\n%s", err, out)
	}
	out, err := p.compose("port", "bastion", "22")
	if err != nil {
		t.Fatalf("compose port: %v\n%s", err, out)
	}
	addr := strings.TrimSpace(out)
	awaitSSHBanner(t, addr)
	return p, addr
}

// awaitSSHBanner waits for sshd to answer. Docker reports a container running before the process
// inside it listens; this is a readiness probe of an external process, not a wait on a goroutine.
func awaitSSHBanner(t *testing.T, addr string) {
	t.Helper()
	deadline := time.Now().Add(startTimeout)
	for time.Now().Before(deadline) {
		c, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
			buf := make([]byte, 8)
			n, _ := c.Read(buf)
			_ = c.Close()
			if strings.HasPrefix(string(buf[:n]), "SSH-2.0") {
				return
			}
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("sshd on %s never answered", addr)
}

// --- seed ---

type hostKey struct {
	signer gossh.Signer
	priv   []byte
	pub    []byte
}

func newHostKey(t *testing.T) hostKey {
	t.Helper()
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(priv)
	if err != nil {
		t.Fatal(err)
	}
	block, err := gossh.MarshalPrivateKey(priv, "")
	if err != nil {
		t.Fatal(err)
	}
	return hostKey{signer: signer, priv: pem.EncodeToMemory(block), pub: gossh.MarshalAuthorizedKey(signer.PublicKey())}
}

func writeSeed(t *testing.T, dir string, files map[string][]byte) {
	t.Helper()
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
}

// --- terminal ---

// terminal collects PTY output and finds markers in it. A marker is computed by the shell -
// "OK-$((40+2))-SSH" prints OK-42-SSH - so the echo of the typed command never matches it.
type terminal struct {
	mu     sync.Mutex
	buf    strings.Builder
	notify chan struct{}
	bridge domain.TerminalPTYBridge
}

func newTerminal(out <-chan []byte, bridge domain.TerminalPTYBridge) *terminal {
	term := &terminal{notify: make(chan struct{}, 1), bridge: bridge}
	go func() {
		for chunk := range out {
			term.mu.Lock()
			term.buf.Write(chunk)
			term.mu.Unlock()
			select {
			case term.notify <- struct{}{}:
			default:
			}
		}
	}()
	return term
}

// run types a command and reports whether it succeeded, by the marker it printed.
func (term *terminal) run(t *testing.T, label, command string) bool {
	t.Helper()
	term.mu.Lock()
	start := term.buf.Len()
	term.mu.Unlock()
	line := fmt.Sprintf("%s && echo OK-$((40+2))-%s || echo FAIL-$((40+2))-%s\n", command, label, label)
	if err := term.bridge.Write([]byte(line)); err != nil {
		t.Fatalf("write to the shell: %v", err)
	}
	ok, fail := "OK-42-"+label, "FAIL-42-"+label
	deadline := time.After(commandTimeout)
	for {
		term.mu.Lock()
		out := term.buf.String()[start:]
		term.mu.Unlock()
		switch {
		case strings.Contains(out, ok):
			return true
		case strings.Contains(out, fail):
			t.Logf("%s failed; shell output:\n%s", label, out)
			return false
		}
		select {
		case <-term.notify:
		case <-deadline:
			t.Fatalf("%s produced no result in %s; output:\n%s", label, commandTimeout, out)
		}
	}
}

// --- application ---

type dialog struct {
	prompts *usecase.AgentConfirmPrompts
	mu      sync.Mutex
	answer  usecase.AgentDecision
	asked   []usecase.AgentConfirmPrompt
}

func (d *dialog) ShowAgentConfirm(p usecase.AgentConfirmPrompt) {
	d.mu.Lock()
	d.asked = append(d.asked, p)
	answer := d.answer
	d.mu.Unlock()
	go func() { _ = d.prompts.Resolve(p.SessionID, p.RequestID, answer) }()
}

func (d *dialog) DismissAgentConfirm(string) {}

func (d *dialog) set(a usecase.AgentDecision) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.answer = a
}

func (d *dialog) snapshot() []usecase.AgentConfirmPrompt {
	d.mu.Lock()
	defer d.mu.Unlock()
	return append([]usecase.AgentConfirmPrompt(nil), d.asked...)
}

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

func (m *memoryAudit) count(prefix string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	n := 0
	for _, l := range m.lines {
		if strings.HasPrefix(l, prefix) {
			n++
		}
	}
	return n
}

type noSFTP struct{}

func (noSFTP) New(domain.SSHClient, int) (domain.RemoteFS, error) { return nil, errors.New("not used") }

type fixture struct {
	dialog   *dialog
	audit    *memoryAudit
	term     *terminal
	targetFP string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	ctx := context.Background()
	vaultRepo := persistence.NewVaultRepo(t.TempDir())
	if err := vaultRepo.Create(ctx, "agent forwarding docker e2e master password"); err != nil {
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
	forwarded, err := keyMgr.Generate(ctx, domain.GeneratedKeySpec{Algorithm: domain.AlgorithmEd25519, Comment: "deploy key"}, "",
		usecase.KeyOptions{AllowForwarding: true})
	if err != nil {
		t.Fatal(err)
	}

	bastionKey, targetKey := newHostKey(t), newHostKey(t)
	seed := t.TempDir()
	writeSeed(t, seed, map[string][]byte{
		"bastion_host_ed25519_key":     bastionKey.priv,
		"bastion_host_ed25519_key.pub": bastionKey.pub,
		"target_host_ed25519_key":      targetKey.priv,
		"target_host_ed25519_key.pub":  targetKey.pub,
		"bastion_known_hosts":          append([]byte("target "), targetKey.pub...),
		"forwarded_key.pub":            []byte(forwarded.PublicKey + "\n"),
	})
	_, addr := startProject(t, seed)
	host, portStr, _ := net.SplitHostPort(addr)
	var port int
	fmt.Sscanf(portStr, "%d", &port)

	for name, key := range map[string]gossh.PublicKey{
		fmt.Sprintf("[%s]:%d", host, port): bastionKey.signer.PublicKey(),
		"target":                           targetKey.signer.PublicKey(),
	} {
		if err := knownHosts.Add(ctx, name, key); err != nil {
			t.Fatal(err)
		}
	}
	pwRef, err := passwords.Import(ctx, []byte("bastion-password"), "bastion")
	if err != nil {
		t.Fatal(err)
	}
	if err := connRepo.Save(ctx, &domain.Connection{
		ID: "bastion", Name: "bastion", Host: host, Port: port, Protocol: domain.ProtocolSSH,
		Users:           []domain.ConnectionUser{{ID: "u", Username: "ops", Auth: domain.AuthMethodPassword, PassAuth: &domain.PasswordAuthConfig{VaultRef: pwRef}}},
		DefaultUserID:   "u",
		AgentForwarding: &agentforward.Config{Enabled: true, IdentityIDs: []string{forwarded.ID}, Confirm: agentforward.ConfirmEach},
	}); err != nil {
		t.Fatal(err)
	}

	f := &fixture{audit: &memoryAudit{}, targetFP: gossh.FingerprintSHA256(targetKey.signer.PublicKey())}
	f.dialog = &dialog{prompts: &usecase.AgentConfirmPrompts{}, answer: usecase.AgentAllowOnce}
	states := make(chan domain.ConnectionSession, 64)
	sessions := usecase.NewSessionManager(usecase.SessionManagerConfig{
		ConnRepo: connRepo, VaultRepo: vaultRepo, IdentRepo: identRepo, PasswordRepo: passwords,
		KnownHosts: knownHosts, SSHFactory: infrassh.NewDialer(), PassphraseCache: cache,
		HostKeyCallbackBuilder: infrassh.NewHostKeyCallbackBuilder(), JumpTransportBuilder: infrassh.NewJumpTransportBuilder(),
		Keys: keyMgr, PTYBridgeFactory: infrassh.NewPTYBridgeFactory(), SFTPClientFactory: noSFTP{},
		ForwardConnLimiterFactory: func() domain.ConcurrencyLimiter { return conlimit.New(4) },
		OnStateChange:             func(s domain.ConnectionSession) { states <- s },
		AgentConfirm: func(ctx context.Context, q usecase.AgentConfirmQuestion) (usecase.AgentDecision, error) {
			return f.dialog.prompts.Ask(ctx, q, f.dialog)
		},
		AgentAudit: f.audit,
	})
	t.Cleanup(sessions.CloseAll)

	sid, err := sessions.OpenSession(ctx, "bastion")
	if err != nil {
		t.Fatal(err)
	}
	for ready := false; !ready; {
		select {
		case s := <-states:
			if s.SessionID == sid && s.State == domain.SessionError {
				t.Fatalf("connecting to the bastion failed: %s", s.ErrorMessage)
			}
			ready = s.SessionID == sid && s.State == domain.SessionReady
		case <-time.After(commandTimeout):
			t.Fatal("the bastion session never became ready")
		}
	}
	out, _, err := sessions.InitSessionIO(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	bridge, err := sessions.GetPTYBridge(sid)
	if err != nil {
		t.Fatal(err)
	}
	f.term = newTerminal(out, bridge)
	return f
}

// --- the scenarios ---

func TestRealOpenSSHThroughTheForwardedAgent(t *testing.T) {
	f := newFixture(t)

	if !f.term.run(t, "LIST", "ssh-add -l") {
		t.Fatal("ssh-add on the bastion does not see the forwarded key")
	}
	if !f.term.run(t, "SSH", "ssh target true") {
		t.Fatal("ssh from the bastion to the target failed")
	}
	if !f.term.run(t, "GIT", "git clone -q git@target:repo.git /tmp/clone && git -C /tmp/clone log --format=%s | grep -qx seed") {
		t.Fatal("git clone of the private repository failed")
	}
	if !f.term.run(t, "SCP", "echo payload >/tmp/f && scp -q /tmp/f target:/tmp/scp-copy") {
		t.Fatal("scp between the servers failed")
	}
	if !f.term.run(t, "RSYNC", "rsync -q -e ssh /tmp/f target:/tmp/rsync-copy && ssh target cat /tmp/rsync-copy | grep -qx payload") {
		t.Fatal("rsync between the servers failed")
	}

	asked := f.dialog.snapshot()
	if len(asked) == 0 {
		t.Fatal("no signature was confirmed with the user")
	}
	for _, q := range asked {
		if !q.Destination.Verified || q.Destination.Fingerprint != f.targetFP {
			t.Fatalf("OpenSSH bound its session, so every request must show the target verified: %+v", q.Destination)
		}
		if len(q.Destination.KnownAs) != 1 || q.Destination.KnownAs[0] != "target" {
			t.Fatalf("the destination must be named by its known_hosts entry: %+v", q.Destination.KnownAs)
		}
		if q.RemoteUser != "ops" && q.RemoteUser != "git" {
			t.Fatalf("remote user %q; the dialog must say whom the server logs in as", q.RemoteUser)
		}
	}
	if allowed := f.audit.count(usecase.AgentEventSignAllowed); allowed != len(asked) {
		t.Fatalf("%d signatures audited for %d approvals", allowed, len(asked))
	}

	if f.term.run(t, "REMOVE", "ssh-add -D 2>/dev/null") {
		t.Fatal("ssh-add -D emptied the forwarded agent")
	}
	if f.audit.count(usecase.AgentEventOpRefused) == 0 {
		t.Fatal("the refused removal is not in the audit log")
	}

	f.dialog.set(usecase.AgentDeny)
	before := len(f.dialog.snapshot())
	if f.term.run(t, "DENIED", "ssh target true") {
		t.Fatal("the target let the bastion in although the user denied the signature")
	}
	if len(f.dialog.snapshot()) == before {
		t.Fatal("the denied login never reached the user")
	}
}
