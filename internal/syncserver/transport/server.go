// Package transport is the sync server's HTTP layer (ADR-023): strict decoding, bearer
// authentication, per-address rate limits, and the mapping of use-case errors onto a closed list
// of error codes. It contains no rule of its own beyond "decode, authenticate, call, encode".
package transport

import (
	"crypto/ed25519"
	"fmt"
	"log/slog"
	"net/http"

	"xquakshell/internal/domain/syncwire"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/syncserver/usecase"
)

// Config holds the transport's limits and the server identity.
type Config struct {
	// IdentitySeed is the server identity key (Ed25519 seed) from the key file.
	IdentitySeed []byte
	// TLSPublicKey is the DER SubjectPublicKeyInfo of the TLS certificate's key.
	TLSPublicKey []byte
	// MaxBody bounds every request body except commits; MaxCommitBody bounds commits.
	MaxBody       int64
	MaxCommitBody int64
	// RatePerSecond and Burst bound requests per client address.
	RatePerSecond float64
	Burst         int
	Logger        *slog.Logger
}

type server struct {
	cfg    Config
	svc    *usecase.Service
	hello  syncwire.HelloReply
	limits *addressLimiter
	log    *slog.Logger
}

// New returns the server's HTTP handler.
func New(cfg Config, svc *usecase.Service) (http.Handler, error) {
	if len(cfg.IdentitySeed) != ed25519.SeedSize {
		return nil, fmt.Errorf("transport: identity seed must be %d bytes", ed25519.SeedSize)
	}
	cfg = withDefaults(cfg)
	identity := ed25519.NewKeyFromSeed(cfg.IdentitySeed)
	defer clear(identity)
	msg, err := vaultsync.ServerTLSMessage(cfg.TLSPublicKey)
	if err != nil {
		return nil, err
	}
	s := &server{cfg: cfg, svc: svc, limits: newAddressLimiter(cfg.RatePerSecond, cfg.Burst), log: cfg.Logger}
	s.hello = syncwire.HelloReply{
		ServerMin: wireVersion(vaultsync.CurrentProtocol), ServerMax: wireVersion(vaultsync.CurrentProtocol),
		Suites:            []uint16{uint16(vaultsync.CurrentSuite)},
		ServerIdentityKey: identity.Public().(ed25519.PublicKey), TLSKeySig: ed25519.Sign(identity, msg),
	}
	return s.routes(), nil
}

func withDefaults(cfg Config) Config {
	if cfg.MaxBody == 0 {
		cfg.MaxBody = 4 << 20
	}
	if cfg.MaxCommitBody == 0 {
		cfg.MaxCommitBody = 20 << 20
	}
	if cfg.RatePerSecond == 0 {
		cfg.RatePerSecond, cfg.Burst = 20, 40
	}
	if cfg.Logger == nil {
		cfg.Logger = slog.Default()
	}
	return cfg
}

func wireVersion(v vaultsync.ProtocolVersion) syncwire.Version {
	return syncwire.Version{Major: v.Major, Minor: v.Minor, Patch: v.Patch}
}

func (s *server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/hello", s.handleHello)
	mux.HandleFunc("POST /v1/register/init", s.handleRegisterInit)
	mux.HandleFunc("POST /v1/register/finish", s.handleRegisterFinish)
	mux.HandleFunc("POST /v1/login/init", s.handleLoginInit)
	mux.HandleFunc("POST /v1/login/finish", s.handleLoginFinish)
	mux.HandleFunc("POST /v1/device/challenge", s.handleDeviceChallenge)
	mux.HandleFunc("POST /v1/device/verify", s.handleDeviceVerify)
	mux.HandleFunc("GET /v1/accounts/lookup", s.handleLookup)
	mux.HandleFunc("GET /v1/accounts/{id}/devicelog", s.handleDeviceLog)
	mux.HandleFunc("POST /v1/account/devicelog", s.handleDeviceLogAppend)
	mux.HandleFunc("GET /v1/vaults", s.handleVaults)
	mux.HandleFunc("POST /v1/vaults", s.handleVaultCreate)
	mux.HandleFunc("GET /v1/vaults/{id}/membership", s.handleMembership)
	mux.HandleFunc("POST /v1/vaults/{id}/membership", s.handleMembershipAppend)
	mux.HandleFunc("POST /v1/vaults/{id}/commit", s.handleCommit)
	mux.HandleFunc("GET /v1/vaults/{id}/changes", s.handleChanges)
	mux.HandleFunc("GET /v1/vaults/{id}/snapshot", s.handleSnapshot)
	mux.HandleFunc("GET /v1/events", s.handleEvents)
	return s.limit(mux)
}

func (s *server) handleHello(w http.ResponseWriter, r *http.Request) {
	var req syncwire.Hello
	if !s.decode(w, r, s.cfg.MaxBody, &req) {
		return
	}
	client := vaultsync.ProtocolVersion{Major: req.ClientProtocol.Major, Minor: req.ClientProtocol.Minor, Patch: req.ClientProtocol.Patch}
	if _, err := vaultsync.NegotiateProtocol(client, vaultsync.CurrentProtocol, vaultsync.CurrentProtocol); err != nil {
		writeCode(w, http.StatusBadRequest, syncwire.CodeUnsupportedVersion)
		return
	}
	writeJSON(w, http.StatusOK, s.hello)
}
