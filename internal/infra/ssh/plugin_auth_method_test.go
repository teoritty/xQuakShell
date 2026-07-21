package ssh

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"xquakshell/internal/domain"
)

type fakeAuthProvider struct {
	prepareErr error
	signErr    error
	answers    []string
}

func (f *fakeAuthProvider) Prepare(context.Context, string, domain.PluginAuthMethod) ([]byte, error) {
	if f.prepareErr != nil {
		return nil, f.prepareErr
	}
	return []byte{0x00, 0x00, 0x00, 0x0b, 0x09, 0x73, 0x73, 0x68, 0x2d, 0x65, 0x64, 0x32, 0x35, 0x35, 0x31, 0x39}, nil
}

func (f *fakeAuthProvider) AnswerChallenge(context.Context, string, domain.PluginAuthMethod, domain.PluginAuthChallenge) ([]string, error) {
	return f.answers, nil
}

func (f *fakeAuthProvider) Sign(context.Context, string, domain.PluginAuthMethod, domain.PluginAuthSignRequest) (domain.PluginAuthSignResult, error) {
	if f.signErr != nil {
		return domain.PluginAuthSignResult{}, f.signErr
	}
	return domain.PluginAuthSignResult{Signature: []byte{1, 2, 3}, SignatureFormat: "ssh-ed25519"}, nil
}

func TestBuildKeyboardInteractiveDelegatesToProvider(t *testing.T) {
	provider := &fakeAuthProvider{answers: []string{"ok"}}
	b := NewPluginAuthMethodBuilder()
	method := domain.PluginAuthMethod{PluginID: "p", AuthMethodID: "m", Kind: domain.AuthProviderKindKeyboardInteractive}
	auth := b.BuildKeyboardInteractive(context.Background(), provider, "attempt", method)
	if auth == nil {
		t.Fatal("expected auth method")
	}
}

func TestBuildPublicKeySignError(t *testing.T) {
	provider := &fakeAuthProvider{signErr: errors.New("sign failed")}
	pubKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := gossh.NewPublicKey(pubKey)
	if err != nil {
		t.Fatal(err)
	}
	method := domain.PluginAuthMethod{PluginID: "p", AuthMethodID: "m", Kind: domain.AuthProviderKindPublicKey}
	signer := &pluginRemoteSigner{
		ctx: context.Background(), provider: provider, attemptID: "attempt", method: method, pub: pub,
	}
	if _, err := signer.Sign(bytes.NewReader(nil), []byte{1, 2, 3}); err == nil {
		t.Fatal("expected sign error")
	}
}

func TestBuildPublicKeyLazyPrepare(t *testing.T) {
	pubBlob := testSSHPublicKeyBlob(t)
	prepareCalls := 0
	provider := &staticKeyProvider{
		blob: pubBlob,
		onPrepare: func() { prepareCalls++ },
	}
	method := domain.PluginAuthMethod{PluginID: "p", AuthMethodID: "m", Kind: domain.AuthProviderKindPublicKey}
	signer := &pluginRemoteSigner{
		ctx: context.Background(), provider: provider, attemptID: "attempt", method: method,
	}
	if prepareCalls != 0 {
		t.Fatalf("Prepare called before PublicKey, got %d", prepareCalls)
	}
	if signer.PublicKey() == nil {
		t.Fatal("expected PublicKey after lazy prepare")
	}
	if prepareCalls != 1 {
		t.Fatalf("Prepare calls = %d, want 1", prepareCalls)
	}
}

func TestSignWithAlgorithmPassesPreferredAlgo(t *testing.T) {
	pubBlob := testSSHPublicKeyBlob(t)
	var got []string
	provider := &staticKeyProvider{
		blob: pubBlob,
		onSign: func(algorithms []string) { got = algorithms },
	}
	method := domain.PluginAuthMethod{PluginID: "p", AuthMethodID: "m", Kind: domain.AuthProviderKindPublicKey}
	signer := &pluginRemoteSigner{
		ctx: context.Background(), provider: provider, attemptID: "attempt", method: method,
	}
	if _, err := signer.SignWithAlgorithm(bytes.NewReader(nil), []byte{1}, "ssh-ed25519"); err != nil {
		t.Fatalf("SignWithAlgorithm: %v", err)
	}
	if len(got) == 0 || got[0] != "ssh-ed25519" {
		t.Fatalf("algorithms = %v, want ssh-ed25519 first", got)
	}
}

func testSSHPublicKeyBlob(t *testing.T) []byte {
	t.Helper()
	pubKey, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pub, err := gossh.NewPublicKey(pubKey)
	if err != nil {
		t.Fatal(err)
	}
	return pub.Marshal()
}

type staticKeyProvider struct {
	blob      []byte
	onPrepare func()
	onSign    func([]string)
}

func (p *staticKeyProvider) Prepare(context.Context, string, domain.PluginAuthMethod) ([]byte, error) {
	if p.onPrepare != nil {
		p.onPrepare()
	}
	return append([]byte(nil), p.blob...), nil
}
func (p *staticKeyProvider) AnswerChallenge(context.Context, string, domain.PluginAuthMethod, domain.PluginAuthChallenge) ([]string, error) {
	return nil, nil
}
func (p *staticKeyProvider) Sign(_ context.Context, _ string, _ domain.PluginAuthMethod, req domain.PluginAuthSignRequest) (domain.PluginAuthSignResult, error) {
	if p.onSign != nil {
		p.onSign(req.Algorithms)
	}
	return domain.PluginAuthSignResult{Signature: []byte{1}, SignatureFormat: "ssh-ed25519"}, nil
}

type countingPrepareProvider struct {
	inner     *fakeAuthProvider
	onPrepare func()
}

func (p *countingPrepareProvider) Prepare(ctx context.Context, attemptID string, method domain.PluginAuthMethod) ([]byte, error) {
	if p.onPrepare != nil {
		p.onPrepare()
	}
	return p.inner.Prepare(ctx, attemptID, method)
}
func (p *countingPrepareProvider) AnswerChallenge(context.Context, string, domain.PluginAuthMethod, domain.PluginAuthChallenge) ([]string, error) {
	return p.inner.AnswerChallenge(context.Background(), "", domain.PluginAuthMethod{}, domain.PluginAuthChallenge{})
}
func (p *countingPrepareProvider) Sign(ctx context.Context, attemptID string, method domain.PluginAuthMethod, req domain.PluginAuthSignRequest) (domain.PluginAuthSignResult, error) {
	return p.inner.Sign(ctx, attemptID, method, req)
}

type algoCaptureProvider struct {
	onSign func([]string)
}

func (p *algoCaptureProvider) Prepare(context.Context, string, domain.PluginAuthMethod) ([]byte, error) {
	return []byte{0x00, 0x00, 0x00, 0x0b, 0x09, 0x73, 0x73, 0x68, 0x2d, 0x65, 0x64, 0x32, 0x35, 0x35, 0x31, 0x39}, nil
}
func (p *algoCaptureProvider) AnswerChallenge(context.Context, string, domain.PluginAuthMethod, domain.PluginAuthChallenge) ([]string, error) {
	return nil, nil
}
func (p *algoCaptureProvider) Sign(_ context.Context, _ string, _ domain.PluginAuthMethod, req domain.PluginAuthSignRequest) (domain.PluginAuthSignResult, error) {
	if p.onSign != nil {
		p.onSign(req.Algorithms)
	}
	return domain.PluginAuthSignResult{Signature: []byte{1}, SignatureFormat: "ssh-rsa"}, nil
}

func TestBuildPublicKeyPrepareError(t *testing.T) {
	provider := &fakeAuthProvider{prepareErr: errors.New("prepare failed")}
	method := domain.PluginAuthMethod{PluginID: "p", AuthMethodID: "m", Kind: domain.AuthProviderKindPublicKey}
	signer := &pluginRemoteSigner{
		ctx: context.Background(), provider: provider, attemptID: "attempt", method: method,
	}
	if _, err := signer.Sign(bytes.NewReader(nil), []byte{1}); err == nil {
		t.Fatal("expected prepare error at sign time")
	}
}
