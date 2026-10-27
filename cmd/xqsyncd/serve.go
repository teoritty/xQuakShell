package main

import (
	"context"
	"crypto/ed25519"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"path/filepath"
	"time"

	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/opaque"
	"xquakshell/internal/pkg/safego"
	"xquakshell/internal/syncserver/store"
	"xquakshell/internal/syncserver/transport"
	"xquakshell/internal/syncserver/usecase"
)

const dbFileName = "xqsyncd.db"

type systemClock struct{}

func (systemClock) NowUnixMilli() uint64 {
	ms := time.Now().UnixMilli()
	if ms < 0 {
		return 0
	}
	return uint64(ms)
}

type serveConfig struct {
	dir      string
	certPath string
	keyPath  string
	log      *slog.Logger
	// personalQuota and teamQuota bound a vault's ciphertext; zero keeps the use case's default.
	personalQuota uint64
	teamQuota     uint64
	// maxRecords bounds the records a vault holds; zero keeps the use case's default.
	maxRecords int
	// maxRequests bounds the requests served at once; zero keeps the transport's default.
	maxRequests int
	// ratePerSecond and burst bound requests per client address; zero keeps the transport's default.
	ratePerSecond float64
	burst         int
	// initFirst runs init --keep-existing with initHosts before serving (serve --init).
	initFirst bool
	initHosts []string
}

// openService opens the store and builds the use cases with the key file's secrets. The OPAQUE
// context is derived from exactly what transport.New advertises in hello, so a client that saw a
// different offer cannot complete a login.
func openService(ctx context.Context, cfg serveConfig, k *keyFile) (*usecase.Service, *store.DB, error) {
	db, err := store.Open(ctx, filepath.Join(cfg.dir, dbFileName))
	if err != nil {
		return nil, nil, err
	}
	identity := ed25519.NewKeyFromSeed(k.IdentitySeed).Public().(ed25519.PublicKey)
	helloCtx, err := vaultsync.HelloContext(identity, vaultsync.CurrentProtocol, vaultsync.CurrentProtocol, []vaultsync.SuiteID{vaultsync.CurrentSuite})
	if err != nil {
		_ = db.Close()
		return nil, nil, err
	}
	ake, err := opaque.NewServer(helloCtx, k.OPAQUEKeyMaterial)
	if err != nil {
		_ = db.Close()
		return nil, nil, fmt.Errorf("xqsyncd: OPAQUE key material: %w", err)
	}
	svc := usecase.New(usecase.Config{Store: db, Clock: systemClock{}, AKE: ake, Random: defaultRand, FakeAccountSecret: k.FakeAccountSecret,
		PersonalQuota: cfg.personalQuota, TeamQuota: cfg.teamQuota, MaxRecords: cfg.maxRecords})
	return svc, db, nil
}

// serve runs the server on ln until ctx ends, then shuts down within ten seconds.
func serve(ctx context.Context, cfg serveConfig, ln net.Listener) error {
	k, err := readKeyFile(cfg.dir)
	if err != nil {
		return err
	}
	svc, db, err := openService(ctx, cfg, k)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()
	cert, spki, err := loadCertificate(cfg.certPath, cfg.keyPath)
	if err != nil {
		return err
	}
	handler, err := transport.New(transport.Config{IdentitySeed: k.IdentitySeed, TLSPublicKey: spki, Logger: cfg.log,
		RatePerSecond: cfg.ratePerSecond, Burst: cfg.burst, MaxConcurrent: cfg.maxRequests}, svc)
	if err != nil {
		return err
	}
	srv := &http.Server{
		Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 60 * time.Second,
		WriteTimeout: 60 * time.Second, IdleTimeout: 120 * time.Second, MaxHeaderBytes: 16 << 10,
		TLSConfig:   &tls.Config{MinVersion: tls.VersionTLS13, Certificates: []tls.Certificate{cert}},
		BaseContext: func(net.Listener) context.Context { return ctx },
	}
	startJanitor(ctx, svc.PruneAll, janitorInterval, cfg.log)
	// The bound address, not the requested one: with port 0 this is how a harness learns the port.
	if cfg.log != nil {
		cfg.log.Info("xqsyncd listening", "addr", ln.Addr().String(),
			"link", connectLink(certHost(cert), portOf(ln.Addr().String()), identityFingerprint(k)))
	}
	done := make(chan error, 1)
	serveTLS(srv, ln, done)
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
	}
	// Reached only once ctx was cancelled: shutdown needs a live context of its own, or it would
	// abort immediately and cut every in-flight commit mid-transaction.
	shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

func serveTLS(srv *http.Server, ln net.Listener, done chan<- error) {
	run := func() {
		err := srv.ServeTLS(ln, "", "")
		if errors.Is(err, http.ErrServerClosed) {
			err = nil
		}
		done <- err
	}
	safego.GoNamed("xqsyncd.serve", run)
}
