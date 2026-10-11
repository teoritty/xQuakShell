package agentfwd

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/rsa"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"sync"
	"testing"

	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/crypto/ssh/agent"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/agentforward"
)

// fakePolicy signs with one key and records what it was asked. It stands where the use case
// stands; what is under test is the protocol boundary in front of it.
type fakePolicy struct {
	key      gossh.Signer
	signErr  error
	mu       sync.Mutex
	refused  []string
	requests []domain.AgentSignRequest
}

func (p *fakePolicy) PublicKeys(context.Context) ([]domain.AgentPublicKey, error) {
	return []domain.AgentPublicKey{{Blob: p.key.PublicKey().Marshal()}}, nil
}

func (p *fakePolicy) Sign(_ context.Context, req domain.AgentSignRequest) (*gossh.Signature, error) {
	p.mu.Lock()
	p.requests = append(p.requests, req)
	p.mu.Unlock()
	if p.signErr != nil {
		return nil, p.signErr
	}
	return p.key.Sign(rand.Reader, req.Data)
}

func (p *fakePolicy) Refuse(_ context.Context, op string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.refused = append(p.refused, op)
}

func (p *fakePolicy) refusals() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]string(nil), p.refused...)
}

func (p *fakePolicy) signCalls() []domain.AgentSignRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	return append([]domain.AgentSignRequest(nil), p.requests...)
}

func newEd25519(t *testing.T) gossh.Signer {
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

type served struct {
	conn     net.Conn
	client   agent.ExtendedAgent
	bindings *agentforward.Bindings
	done     chan error
}

// serve runs Serve on one end of a pipe and returns an x/crypto agent client on the other: the
// very client OpenSSH-compatible Go tools use, so the protocol is checked against a real peer.
func serve(t *testing.T, policy domain.AgentSignPolicy) *served {
	t.Helper()
	s := serveRaw(t, policy)
	s.client = agent.NewClient(s.conn)
	return s
}

// serveRaw leaves the client end bare for tests that write frames by hand. It cannot also carry
// an agent.Client: that client reads its connection from a goroutine of its own and would consume
// the replies a raw test is waiting for.
func serveRaw(t *testing.T, policy domain.AgentSignPolicy) *served {
	t.Helper()
	a, b := net.Pipe()
	s := &served{conn: b, bindings: &agentforward.Bindings{}, done: make(chan error, 1)}
	go func() { s.done <- Serve(context.Background(), a, policy, s.bindings) }()
	t.Cleanup(func() { _ = a.Close(); _ = b.Close() })
	return s
}

func TestServeListsKeysWithoutComments(t *testing.T) {
	p := &fakePolicy{key: newEd25519(t)}
	keys, err := serve(t, p).client.List()
	if err != nil || len(keys) != 1 {
		t.Fatalf("List = %v, %v", keys, err)
	}
	if !bytes.Equal(keys[0].Blob, p.key.PublicKey().Marshal()) || keys[0].Comment != "" {
		t.Fatalf("listed %q / %q; the key must be listed with no label", keys[0].Format, keys[0].Comment)
	}
}

func TestServePassesSignRequestsThroughAndReturnsTheSignature(t *testing.T) {
	p := &fakePolicy{key: newEd25519(t)}
	s := serve(t, p)
	data := []byte("login data")
	sig, err := s.client.SignWithFlags(p.key.PublicKey(), data, agent.SignatureFlagRsaSha512)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.key.PublicKey().Verify(data, sig); err != nil {
		t.Fatalf("the returned signature does not verify: %v", err)
	}
	reqs := p.signCalls()
	if len(reqs) != 1 || !bytes.Equal(reqs[0].Data, data) || reqs[0].Flags != agentforward.FlagRSASHA512 || reqs[0].Bindings != s.bindings {
		t.Fatalf("the policy must receive the data, the flags and this channel's bindings: %+v", reqs)
	}
}

func TestServeReportsAPolicyRefusalAsFailure(t *testing.T) {
	p := &fakePolicy{key: newEd25519(t), signErr: domain.ErrAgentSignDenied}
	s := serveRaw(t, p)
	frame := gossh.Marshal(signRequestMsg{KeyBlob: p.key.PublicKey().Marshal(), Data: []byte("x")})
	if reply := rawRequest(t, s.conn, frame); !bytes.Equal(reply, []byte{agentFailure}) {
		t.Fatalf("reply %v; a refused signature must reach the remote as SSH_AGENT_FAILURE", reply)
	}
}

// The remote side must not be able to change the agent in any way.
func TestServeRefusesEveryMutatingOperation(t *testing.T) {
	p := &fakePolicy{key: newEd25519(t)}
	s := serve(t, p)
	_, priv, _ := ed25519.GenerateKey(rand.Reader)
	ops := map[string]func() error{
		"add-identity":          func() error { return s.client.Add(agent.AddedKey{PrivateKey: priv}) },
		"remove-identity":       func() error { return s.client.Remove(p.key.PublicKey()) },
		"remove-all-identities": func() error { return s.client.RemoveAll() },
		"lock":                  func() error { return s.client.Lock([]byte("pw")) },
		"unlock":                func() error { return s.client.Unlock([]byte("pw")) },
	}
	for op, call := range ops {
		if err := call(); err == nil {
			t.Errorf("%s succeeded; the forwarded agent is read-only", op)
		}
	}
	got := map[string]bool{}
	for _, r := range p.refusals() {
		got[r] = true
	}
	for op := range ops {
		if !got[op] {
			t.Errorf("%s was not audited as refused (refusals: %v)", op, p.refusals())
		}
	}
	if keys, err := s.client.List(); err != nil || len(keys) != 1 {
		t.Fatal("after every refused mutation the agent must still list exactly its key")
	}
}

func rawRequest(t *testing.T, conn io.ReadWriter, frame []byte) []byte {
	t.Helper()
	if err := writeFrame(conn, frame); err != nil {
		t.Fatal(err)
	}
	reply, err := readFrame(conn)
	if err != nil {
		t.Fatalf("no reply: %v", err)
	}
	return reply
}

func TestServeAnswersUnknownMessagesWithFailure(t *testing.T) {
	p := &fakePolicy{key: newEd25519(t)}
	s := serveRaw(t, p)
	if reply := rawRequest(t, s.conn, []byte{99, 1, 2, 3}); !bytes.Equal(reply, []byte{agentFailure}) {
		t.Fatalf("reply %v, want failure", reply)
	}
	if r := p.refusals(); len(r) != 1 || r[0] != "unknown" {
		t.Fatalf("refusals %v; an unknown message must be audited", r)
	}
	if reply := rawRequest(t, s.conn, []byte{agentRequestIdentities, 0}); !bytes.Equal(reply, []byte{agentFailure}) {
		t.Fatal("a list request with trailing bytes must fail")
	}
}

func TestServeRefusesAMalformedSignRequestBeforeThePolicy(t *testing.T) {
	p := &fakePolicy{key: newEd25519(t)}
	s := serveRaw(t, p)
	frame := append(gossh.Marshal(signRequestMsg{KeyBlob: p.key.PublicKey().Marshal(), Data: []byte("d")}), 0xff)
	if reply := rawRequest(t, s.conn, frame); !bytes.Equal(reply, []byte{agentFailure}) {
		t.Fatalf("reply %v, want failure", reply)
	}
	if len(p.signCalls()) != 0 {
		t.Fatal("a malformed request must not reach the policy")
	}
}

func TestServeEndsTheChannelOnAnUntrustworthyLength(t *testing.T) {
	for name, n := range map[string]uint32{"zero": 0, "one past the limit": agentforward.MaxFrameLen + 1, "huge": 0xffffffff} {
		t.Run(name, func(t *testing.T) {
			p := &fakePolicy{key: newEd25519(t)}
			s := serveRaw(t, p)
			var hdr [4]byte
			binary.BigEndian.PutUint32(hdr[:], n)
			go func() { _, _ = s.conn.Write(hdr[:]) }()
			if err := <-s.done; !errors.Is(err, errFrame) {
				t.Fatalf("Serve = %v; a frame of length %d must end the channel", err, n)
			}
			if r := p.refusals(); len(r) != 1 || r[0] != "bad-frame" {
				t.Fatalf("refusals %v; a bad frame must be audited", r)
			}
		})
	}
}

func TestServeAcceptsAFrameOfExactlyTheLimit(t *testing.T) {
	p := &fakePolicy{key: newEd25519(t)}
	s := serveRaw(t, p)
	frame := make([]byte, agentforward.MaxFrameLen)
	frame[0] = 99
	if reply := rawRequest(t, s.conn, frame); !bytes.Equal(reply, []byte{agentFailure}) {
		t.Fatalf("a frame of exactly MaxFrameLen must be read and answered, got %v", reply)
	}
}

func TestServeEndsWhenThePeerHangsUp(t *testing.T) {
	s := serveRaw(t, &fakePolicy{key: newEd25519(t)})
	_ = s.conn.Close()
	if err := <-s.done; !errors.Is(err, io.EOF) && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("Serve = %v; a closed channel must end the loop", err)
	}
}

// --- session-bind ---

var bindSessionID = []byte("0123456789abcdef0123456789abcdef")

func bindContents(t *testing.T, hostKey, signer gossh.Signer, sid []byte, forwarding bool) []byte {
	t.Helper()
	sig, err := signer.Sign(rand.Reader, sid)
	if err != nil {
		t.Fatal(err)
	}
	return gossh.Marshal(sessionBindMsg{
		HostKey: hostKey.PublicKey().Marshal(), SessionID: sid, Signature: gossh.Marshal(sig), Forwarding: forwarding,
	})
}

func TestSessionBindRecordsAVerifiedDestination(t *testing.T) {
	p := &fakePolicy{key: newEd25519(t)}
	s := serve(t, p)
	host := newEd25519(t)
	if _, err := s.client.Extension(sessionBindExtension, bindContents(t, host, host, bindSessionID, true)); err != nil {
		t.Fatalf("a correctly signed session-bind was refused: %v", err)
	}
	b, ok := s.bindings.Lookup(bindSessionID)
	if !ok || b.HostKeyFingerprint != gossh.FingerprintSHA256(host.PublicKey()) || !b.Forwarding {
		t.Fatalf("binding = %+v, %v; want the host's fingerprint and the forwarding flag", b, ok)
	}
}

func TestSessionBindRefusesForgeries(t *testing.T) {
	host := newEd25519(t)
	attacker := newEd25519(t)
	rsaKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	rsaSigner, _ := gossh.NewSignerFromKey(rsaKey)
	sha1Sig, err := rsaSigner.(gossh.AlgorithmSigner).SignWithAlgorithm(rand.Reader, bindSessionID, gossh.KeyAlgoRSA)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string][]byte{
		"signed by another key": bindContents(t, host, attacker, bindSessionID, false),
		"signature over another session": gossh.Marshal(sessionBindMsg{
			HostKey: host.PublicKey().Marshal(), SessionID: bindSessionID,
			Signature: gossh.Marshal(mustSign(t, host, []byte("other session"))),
		}),
		"sha-1 rsa": gossh.Marshal(sessionBindMsg{
			HostKey: rsaSigner.PublicKey().Marshal(), SessionID: bindSessionID, Signature: gossh.Marshal(sha1Sig),
		}),
		"garbage host key": gossh.Marshal(sessionBindMsg{HostKey: []byte("nope"), SessionID: bindSessionID, Signature: []byte("x")}),
		"truncated":        {0, 0, 0, 9},
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			p := &fakePolicy{key: newEd25519(t)}
			s := serve(t, p)
			if _, err := s.client.Extension(sessionBindExtension, contents); err == nil {
				t.Fatal("a forged session-bind was accepted")
			}
			if _, ok := s.bindings.Lookup(bindSessionID); ok {
				t.Fatal("a forged session-bind was recorded")
			}
			if r := p.refusals(); len(r) != 1 || r[0] != "session-bind" {
				t.Fatalf("refusals %v; a forged bind must be audited", r)
			}
		})
	}
}

func TestSessionBindRefusesRebindingToAnotherHost(t *testing.T) {
	s := serve(t, &fakePolicy{key: newEd25519(t)})
	hostB, evil := newEd25519(t), newEd25519(t)
	if _, err := s.client.Extension(sessionBindExtension, bindContents(t, hostB, hostB, bindSessionID, false)); err != nil {
		t.Fatal(err)
	}
	if _, err := s.client.Extension(sessionBindExtension, bindContents(t, evil, evil, bindSessionID, false)); err == nil {
		t.Fatal("a second host claiming a bound session was accepted")
	}
	if b, _ := s.bindings.Lookup(bindSessionID); b.HostKeyFingerprint != gossh.FingerprintSHA256(hostB.PublicKey()) {
		t.Fatal("the original binding must survive a refused rebind")
	}
}

func TestServeAnswersUnsupportedExtensionsWithPlainFailure(t *testing.T) {
	p := &fakePolicy{key: newEd25519(t)}
	s := serveRaw(t, p)
	frame := gossh.Marshal(extensionMsg{ExtensionType: "query"})
	if reply := rawRequest(t, s.conn, frame); !bytes.Equal(reply, []byte{agentFailure}) {
		t.Fatalf("reply %v; PROTOCOL.agent wants SSH_AGENT_FAILURE for an unsupported extension", reply)
	}
}

func TestSessionBindTakesTheCertifiedKeysFingerprint(t *testing.T) {
	ca, host := newEd25519(t), newEd25519(t)
	cert := &gossh.Certificate{Key: host.PublicKey(), CertType: gossh.HostCert, ValidPrincipals: []string{"git.corp"}, ValidBefore: gossh.CertTimeInfinity}
	if err := cert.SignCert(rand.Reader, ca); err != nil {
		t.Fatal(err)
	}
	certSigner, err := gossh.NewCertSigner(cert, host)
	if err != nil {
		t.Fatal(err)
	}
	b, err := verifySessionBind(bindContents(t, certSigner, host, bindSessionID, false))
	if err != nil {
		t.Fatalf("a host presenting a certificate must be bindable: %v", err)
	}
	if b.HostKeyFingerprint != gossh.FingerprintSHA256(host.PublicKey()) {
		t.Fatal("the fingerprint shown must be the certified host key's, which known_hosts names")
	}
}

func mustSign(t *testing.T, s gossh.Signer, data []byte) *gossh.Signature {
	t.Helper()
	sig, err := s.Sign(rand.Reader, data)
	if err != nil {
		t.Fatal(err)
	}
	return sig
}
