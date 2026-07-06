package ssh

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"errors"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"ssh-client/internal/domain"
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

func TestBuildPublicKeyPrepareError(t *testing.T) {
	provider := &fakeAuthProvider{prepareErr: errors.New("prepare failed")}
	b := NewPluginAuthMethodBuilder()
	method := domain.PluginAuthMethod{PluginID: "p", AuthMethodID: "m", Kind: domain.AuthProviderKindPublicKey}
	if _, err := b.BuildPublicKey(context.Background(), provider, "attempt", method); err == nil {
		t.Fatal("expected prepare error")
	}
}
