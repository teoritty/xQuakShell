package ssh

import (
	"context"
	"fmt"
	"io"

	gossh "golang.org/x/crypto/ssh"

	"ssh-client/internal/domain"
)

type pluginAuthMethodBuilder struct{}

// NewPluginAuthMethodBuilder returns a domain.PluginAuthMethodBuilder implementation.
func NewPluginAuthMethodBuilder() domain.PluginAuthMethodBuilder {
	return pluginAuthMethodBuilder{}
}

func (pluginAuthMethodBuilder) BuildKeyboardInteractive(
	ctx context.Context,
	provider domain.PluginAuthProvider,
	attemptID string,
	method domain.PluginAuthMethod,
) domain.AuthMethod {
	return gossh.KeyboardInteractive(func(name, instruction string, questions []string, echos []bool) ([]string, error) {
		qs := make([]domain.PluginAuthQuestion, len(questions))
		for i := range questions {
			echo := false
			if i < len(echos) {
				echo = echos[i]
			}
			qs[i] = domain.PluginAuthQuestion{Text: questions[i], EchoOn: echo}
		}
		return provider.AnswerChallenge(ctx, attemptID, method, domain.PluginAuthChallenge{
			Name: name, Instruction: instruction, Questions: qs,
		})
	})
}

func (pluginAuthMethodBuilder) BuildPublicKey(
	ctx context.Context,
	provider domain.PluginAuthProvider,
	attemptID string,
	method domain.PluginAuthMethod,
) (domain.AuthMethod, error) {
	blob, err := provider.Prepare(ctx, attemptID, method)
	if err != nil {
		return nil, fmt.Errorf("plugin auth prepare: %w", err)
	}
	pub, err := gossh.ParsePublicKey(blob)
	if err != nil {
		return nil, fmt.Errorf("parse plugin public key: %w", err)
	}
	signer := &pluginRemoteSigner{
		ctx: ctx, provider: provider, attemptID: attemptID, method: method, pub: pub,
	}
	return gossh.PublicKeys(signer), nil
}

type pluginRemoteSigner struct {
	ctx       context.Context
	provider  domain.PluginAuthProvider
	attemptID string
	method    domain.PluginAuthMethod
	pub       gossh.PublicKey
}

func (s *pluginRemoteSigner) PublicKey() gossh.PublicKey { return s.pub }

func (s *pluginRemoteSigner) Sign(_ io.Reader, data []byte) (*gossh.Signature, error) {
	res, err := s.provider.Sign(s.ctx, s.attemptID, s.method, domain.PluginAuthSignRequest{
		DataToSign: data,
		Algorithms: []string{s.pub.Type()},
	})
	if err != nil {
		return nil, fmt.Errorf("plugin remote sign: %w", err)
	}
	return &gossh.Signature{Format: res.SignatureFormat, Blob: res.Signature}, nil
}

var _ gossh.Signer = (*pluginRemoteSigner)(nil)
