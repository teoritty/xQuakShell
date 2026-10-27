package syncclient

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/syncwire"
)

const (
	defaultReplyLimit = 1 << 20
	// bulkReplyLimit bounds a page of changes or a snapshot. The server caps one commit at 20 MiB.
	bulkReplyLimit  = 64 << 20
	errorReplyLimit = 4 << 10
	maxRetryAfter   = time.Hour
)

// call is one request. A zero limit means defaultReplyLimit.
type call struct {
	client *http.Client
	method string
	path   string
	query  url.Values
	body   any
	reply  any
	auth   bool
	limit  int64
}

// do sends one request and decodes the reply strictly. It returns the SPKI of the connection the
// reply came on, which Hello needs to bind the identity signature to this very connection.
func (c *Client) do(ctx context.Context, k call) ([]byte, error) {
	req, err := c.newRequest(ctx, k)
	if err != nil {
		return nil, err
	}
	resp, err := k.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("syncclient: %s %s: %w", k.method, k.path, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.TLS == nil || len(resp.TLS.PeerCertificates) == 0 {
		return nil, synctrust.ErrServerIdentity
	}
	spki := resp.TLS.PeerCertificates[0].RawSubjectPublicKeyInfo
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, remoteError(resp)
	}
	limit := k.limit
	if limit == 0 {
		limit = defaultReplyLimit
	}
	if err := syncwire.Decode(resp.Body, limit, k.reply); err != nil {
		return nil, fmt.Errorf("%w: %s %s: %w", synctrust.ErrRemoteProtocol, k.method, k.path, err)
	}
	return spki, nil
}

func (c *Client) newRequest(ctx context.Context, k call) (*http.Request, error) {
	u := c.base.JoinPath(k.path)
	u.RawQuery = k.query.Encode()
	var body io.Reader
	if k.body != nil {
		raw, err := json.Marshal(k.body)
		if err != nil {
			return nil, fmt.Errorf("syncclient: encode %s: %w", k.path, err)
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, k.method, u.String(), body)
	if err != nil {
		return nil, fmt.Errorf("syncclient: build %s: %w", k.path, err)
	}
	if k.body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if k.auth {
		c.mu.Lock()
		token := c.access
		c.mu.Unlock()
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	return req, nil
}

var codeErrors = map[string]error{
	syncwire.CodeMalformed:          synctrust.ErrRemoteMalformed,
	syncwire.CodeUnauthorized:       synctrust.ErrRemoteUnauthorized,
	syncwire.CodeForbidden:          synctrust.ErrRemoteForbidden,
	syncwire.CodeNotFound:           synctrust.ErrRemoteNotFound,
	syncwire.CodeTooLarge:           synctrust.ErrRemoteTooLarge,
	syncwire.CodeUnsupportedVersion: synctrust.ErrRemoteUnsupported,
	syncwire.CodeFrozen:             synctrust.ErrRemoteFrozen,
	syncwire.CodeQuotaExceeded:      synctrust.ErrRemoteQuota,
	syncwire.CodeInternal:           synctrust.ErrRemoteInternal,
	syncwire.CodeHistoryPruned:      synctrust.ErrHistoryPruned,
	syncwire.CodePurgeNotDue:        synctrust.ErrPurgeNotDue,
}

// remoteError maps an error reply onto a domain error. Only the closed set of codes is accepted; a
// body with anything else - text, a stack trace, an unknown code - is a protocol violation.
func remoteError(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusConflict:
		var body syncwire.ConflictReply
		if err := syncwire.Decode(resp.Body, errorReplyLimit, &body); err != nil || body.Error != syncwire.CodeConflict {
			return fmt.Errorf("%w: conflict reply", synctrust.ErrRemoteProtocol)
		}
		return &synctrust.ConflictError{CurrentSeq: body.CurrentSeq}
	case http.StatusTooManyRequests:
		return &synctrust.RateLimitedError{RetryAfter: retryAfter(resp.Header.Get("Retry-After"))}
	}
	if resp.StatusCode < 400 {
		return fmt.Errorf("%w: status %d", synctrust.ErrRemoteProtocol, resp.StatusCode)
	}
	var body syncwire.ErrorBody
	if err := syncwire.Decode(resp.Body, errorReplyLimit, &body); err != nil {
		return fmt.Errorf("%w: status %d with an undecodable body", synctrust.ErrRemoteProtocol, resp.StatusCode)
	}
	if mapped, ok := codeErrors[body.Error]; ok {
		return mapped
	}
	return fmt.Errorf("%w: unknown error code", synctrust.ErrRemoteProtocol)
}

// retryAfter reads whole seconds, defaulting to one and capped at an hour, so a hostile server
// cannot park sync forever with one header.
func retryAfter(h string) time.Duration {
	secs, err := strconv.Atoi(h)
	if err != nil || secs < 1 {
		return time.Second
	}
	d := time.Duration(secs) * time.Second
	if d > maxRetryAfter {
		return maxRetryAfter
	}
	return d
}
