package ssh

import (
	"context"
	"fmt"
	"io"
	"sync"

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
	signer := &pluginRemoteSigner{
		ctx: ctx, provider: provider, attemptID: attemptID, method: method,
	}
	return gossh.PublicKeys(signer), nil
}

type pluginRemoteSigner struct {
	ctx       context.Context
	provider  domain.PluginAuthProvider
	attemptID string
	method    domain.PluginAuthMethod
	once      sync.Once
	pub       gossh.PublicKey
	prepareErr error
}

func (s *pluginRemoteSigner) ensurePrepared() error {
	s.once.Do(func() {
		blob, err := s.provider.Prepare(s.ctx, s.attemptID, s.method)
		if err != nil {
			s.prepareErr = fmt.Errorf("plugin auth prepare: %w", err)
			return
		}
		s.pub, s.prepareErr = gossh.ParsePublicKey(blob)
		if s.prepareErr != nil {
			s.prepareErr = fmt.Errorf("parse plugin public key: %w", s.prepareErr)
		}
	})
	return s.prepareErr
}

func (s *pluginRemoteSigner) PublicKey() gossh.PublicKey {
	if err := s.ensurePrepared(); err != nil {
		return nil
	}
	return s.pub
}

func (s *pluginRemoteSigner) Sign(rand io.Reader, data []byte) (*gossh.Signature, error) {
	return s.SignWithAlgorithm(rand, data, "")
}

func (s *pluginRemoteSigner) SignWithAlgorithm(_ io.Reader, data []byte, algorithm string) (*gossh.Signature, error) {
	if err := s.ensurePrepared(); err != nil {
		return nil, err
	}
	algorithms := []string{s.pub.Type()}
	if algorithm != "" {
		algorithms = []string{algorithm, s.pub.Type()}
	}
	res, err := s.provider.Sign(s.ctx, s.attemptID, s.method, domain.PluginAuthSignRequest{
		DataToSign: data,
		Algorithms: algorithms,
	})
	if err != nil {
		return nil, fmt.Errorf("plugin remote sign: %w", err)
	}
	return &gossh.Signature{Format: res.SignatureFormat, Blob: res.Signature}, nil
}

var (
	_ gossh.Signer          = (*pluginRemoteSigner)(nil)
	_ gossh.AlgorithmSigner = (*pluginRemoteSigner)(nil)
)
