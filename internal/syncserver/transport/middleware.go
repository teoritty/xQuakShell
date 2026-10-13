package transport

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"golang.org/x/time/rate"

	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/usecase"
)

const maxTrackedAddresses = 50_000

// addressLimiter is a token bucket per client address. The table is swept when it grows past a
// bound, so spraying addresses cannot exhaust memory.
type addressLimiter struct {
	mu    sync.Mutex
	r     rate.Limit
	burst int
	m     map[string]*rate.Limiter
}

func newAddressLimiter(perSecond float64, burst int) *addressLimiter {
	return &addressLimiter{r: rate.Limit(perSecond), burst: burst, m: map[string]*rate.Limiter{}}
}

func (l *addressLimiter) allow(addr string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	lim, ok := l.m[addr]
	if !ok {
		if len(l.m) >= maxTrackedAddresses {
			l.m = map[string]*rate.Limiter{}
		}
		lim = rate.NewLimiter(l.r, l.burst)
		l.m[addr] = lim
	}
	return lim.Allow()
}

// clientAddress is the TCP peer. Forwarded headers are not trusted: a client could write any
// address there and dodge both the rate limit and the login backoff.
func clientAddress(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *server) limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !s.limits.allow(clientAddress(r)) {
			w.Header().Set("Retry-After", "1")
			writeCode(w, http.StatusTooManyRequests, syncwire.CodeRateLimited)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// decode reads a strict JSON body; it writes the error response itself and reports whether the
// handler may continue.
func (s *server) decode(w http.ResponseWriter, r *http.Request, limit int64, v any) bool {
	body := http.MaxBytesReader(w, r.Body, limit)
	err := syncwire.Decode(body, limit, v)
	if err == nil {
		return true
	}
	var tooBig *http.MaxBytesError
	if errors.As(err, &tooBig) || errors.Is(err, syncwire.ErrTooLarge) {
		writeCode(w, http.StatusRequestEntityTooLarge, syncwire.CodeTooLarge)
		return false
	}
	writeCode(w, http.StatusBadRequest, syncwire.CodeMalformed)
	return false
}

// authenticate resolves the bearer token for one of the given kinds, writing the error itself.
func (s *server) authenticate(w http.ResponseWriter, r *http.Request, kinds ...usecase.TokenKind) (usecase.Principal, bool) {
	token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || token == "" {
		writeCode(w, http.StatusUnauthorized, syncwire.CodeUnauthorized)
		return usecase.Principal{}, false
	}
	p, err := s.svc.Authenticate(r.Context(), token, kinds...)
	if err != nil {
		s.fail(w, r, err)
		return usecase.Principal{}, false
	}
	return p, true
}

// fail maps a use-case error onto a status and a code. Only the code reaches the client; the error
// text, which may name tables or ids, goes to the server log alone.
func (s *server) fail(w http.ResponseWriter, r *http.Request, err error) {
	var conflict *usecase.ConflictError
	var limited *usecase.RateLimitedError
	switch {
	case errors.As(err, &conflict):
		writeJSON(w, http.StatusConflict, syncwire.ConflictReply{Error: syncwire.CodeConflict, CurrentSeq: conflict.CurrentSeq})
	case errors.As(err, &limited):
		w.Header().Set("Retry-After", strconv.FormatUint((limited.RetryAfterMillis+999)/1000, 10))
		writeCode(w, http.StatusTooManyRequests, syncwire.CodeRateLimited)
	case errors.Is(err, usecase.ErrMalformed):
		writeCode(w, http.StatusBadRequest, syncwire.CodeMalformed)
	case errors.Is(err, usecase.ErrUnauthorized):
		writeCode(w, http.StatusUnauthorized, syncwire.CodeUnauthorized)
	case errors.Is(err, usecase.ErrFrozen):
		writeCode(w, http.StatusForbidden, syncwire.CodeFrozen)
	case errors.Is(err, usecase.ErrForbidden):
		writeCode(w, http.StatusForbidden, syncwire.CodeForbidden)
	case errors.Is(err, usecase.ErrNotFound):
		writeCode(w, http.StatusNotFound, syncwire.CodeNotFound)
	case errors.Is(err, usecase.ErrQuotaExceeded):
		writeCode(w, http.StatusInsufficientStorage, syncwire.CodeQuotaExceeded)
	case errors.Is(err, usecase.ErrTooLarge):
		writeCode(w, http.StatusRequestEntityTooLarge, syncwire.CodeTooLarge)
	default:
		s.log.Error("sync request failed", "method", r.Method, "path", r.URL.Path, "err", err)
		writeCode(w, http.StatusInternalServerError, syncwire.CodeInternal)
	}
}

func writeCode(w http.ResponseWriter, status int, code string) {
	writeJSON(w, status, syncwire.ErrorBody{Error: code})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v) // #nosec G104 -- the status is already sent; a failed write has no one left to tell
}

// pathID parses the {id} segment; a malformed id is reported as not found, like an unknown one.
func pathID(w http.ResponseWriter, r *http.Request) (vaultsync.ID, bool) {
	id, err := syncwire.ParseID(r.PathValue("id"))
	if err != nil {
		writeCode(w, http.StatusNotFound, syncwire.CodeNotFound)
		return id, false
	}
	return id, true
}

func queryUint(r *http.Request, name string) uint64 {
	v, err := strconv.ParseUint(r.URL.Query().Get(name), 10, 64)
	if err != nil {
		return 0
	}
	return v
}
