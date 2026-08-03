package usecase

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
)

const (
	authPrepareTimeout   = 30 * time.Second
	authChallengeTimeout = 120 * time.Second
	authSignTimeout      = 10 * time.Second
)

// PluginCaller is the minimal surface PluginAuthBridge needs from PluginManager.
type PluginCaller interface {
	CallWithTimeout(ctx context.Context, pluginID, method string, params json.RawMessage, timeout time.Duration) (json.RawMessage, error)
}

// PluginAuthBridge implements domain.PluginAuthProvider over the plugin RPC transport.
type PluginAuthBridge struct {
	caller     PluginCaller
	authorizer domainplugin.AuthAttemptAuthorizer
}

// NewPluginAuthBridge creates a plugin auth RPC bridge.
func NewPluginAuthBridge(caller PluginCaller, authorizer domainplugin.AuthAttemptAuthorizer) *PluginAuthBridge {
	return &PluginAuthBridge{caller: caller, authorizer: authorizer}
}

type authPrepareParams struct {
	AttemptID    string            `json:"attemptId"`
	AuthMethodID string            `json:"authMethodId"`
	ConnectionID string            `json:"connectionId"`
	Fields       map[string]string `json:"fields,omitempty"`
}
type authPrepareResult struct {
	PublicKeyBlobBase64 string `json:"publicKeyBlobBase64"`
}

func (b *PluginAuthBridge) Prepare(ctx context.Context, attemptID string, method domain.PluginAuthMethod) ([]byte, error) {
	if err := b.authorizeAttempt(method, attemptID); err != nil {
		return nil, err
	}
	params, _ := json.Marshal(authPrepareParams{
		AttemptID: attemptID, AuthMethodID: method.AuthMethodID,
		ConnectionID: method.ConnectionID, Fields: method.Fields,
	})
	raw, err := b.caller.CallWithTimeout(ctx, method.PluginID, "auth.prepare", params, authPrepareTimeout)
	if err != nil {
		return nil, err
	}
	var res authPrepareResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("auth.prepare: decode result: %w", err)
	}
	return base64.StdEncoding.DecodeString(res.PublicKeyBlobBase64)
}

type authChallengeQuestionWire struct {
	Text   string `json:"text"`
	EchoOn bool   `json:"echoOn"`
}
type authChallengeParams struct {
	AttemptID    string                      `json:"attemptId"`
	AuthMethodID string                      `json:"authMethodId"`
	Name         string                      `json:"name"`
	Instruction  string                      `json:"instruction"`
	Questions    []authChallengeQuestionWire `json:"questions"`
}
type authChallengeResult struct {
	Answers []string `json:"answers"`
}

func (b *PluginAuthBridge) AnswerChallenge(ctx context.Context, attemptID string, method domain.PluginAuthMethod, ch domain.PluginAuthChallenge) ([]string, error) {
	if err := b.authorizeAttempt(method, attemptID); err != nil {
		return nil, err
	}
	qs := make([]authChallengeQuestionWire, len(ch.Questions))
	for i, q := range ch.Questions {
		qs[i] = authChallengeQuestionWire{Text: q.Text, EchoOn: q.EchoOn}
	}
	params, _ := json.Marshal(authChallengeParams{
		AttemptID: attemptID, AuthMethodID: method.AuthMethodID,
		Name: ch.Name, Instruction: ch.Instruction, Questions: qs,
	})
	raw, err := b.caller.CallWithTimeout(ctx, method.PluginID, "auth.answerChallenge", params, authChallengeTimeout)
	if err != nil {
		if errors.Is(err, domainplugin.ErrAuthChallengeTimeout) || errors.Is(err, context.DeadlineExceeded) {
			return nil, domainplugin.ErrAuthChallengeTimeout
		}
		return nil, err
	}
	var res authChallengeResult
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("auth.answerChallenge: decode result: %w", err)
	}
	return res.Answers, nil
}

type authSignParams struct {
	AttemptID    string   `json:"attemptId"`
	AuthMethodID string   `json:"authMethodId"`
	DataBase64   string   `json:"dataBase64"`
	Algorithms   []string `json:"algorithms"`
}
type authSignResultWire struct {
	SignatureBase64 string `json:"signatureBase64"`
	SignatureFormat string `json:"signatureFormat"`
}

func (b *PluginAuthBridge) Sign(ctx context.Context, attemptID string, method domain.PluginAuthMethod, req domain.PluginAuthSignRequest) (domain.PluginAuthSignResult, error) {
	if err := b.authorizeAttempt(method, attemptID); err != nil {
		return domain.PluginAuthSignResult{}, err
	}
	params, _ := json.Marshal(authSignParams{
		AttemptID: attemptID, AuthMethodID: method.AuthMethodID,
		DataBase64: base64.StdEncoding.EncodeToString(req.DataToSign),
		Algorithms: req.Algorithms,
	})
	raw, err := b.caller.CallWithTimeout(ctx, method.PluginID, "auth.sign", params, authSignTimeout)
	if err != nil {
		return domain.PluginAuthSignResult{}, err
	}
	var res authSignResultWire
	if err := json.Unmarshal(raw, &res); err != nil {
		return domain.PluginAuthSignResult{}, fmt.Errorf("auth.sign: decode result: %w", err)
	}
	sig, err := base64.StdEncoding.DecodeString(res.SignatureBase64)
	if err != nil {
		return domain.PluginAuthSignResult{}, fmt.Errorf("auth.sign: decode signature: %w", err)
	}
	return domain.PluginAuthSignResult{Signature: sig, SignatureFormat: res.SignatureFormat}, nil
}

func (b *PluginAuthBridge) authorizeAttempt(method domain.PluginAuthMethod, attemptID string) error {
	if b.authorizer == nil {
		return fmt.Errorf("auth attempt authorizer unavailable")
	}
	return b.authorizer.Authorize(method.PluginID, attemptID, method.AuthMethodID, method.ConnectionID)
}

var _ domain.PluginAuthProvider = (*PluginAuthBridge)(nil)
