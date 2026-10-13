package main

import (
	"context"
	"crypto/rand"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"time"

	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
	"xquakshell/internal/infra/opaque"
	"xquakshell/internal/infra/persistence"
	"xquakshell/internal/infra/syncclient"
	"xquakshell/internal/infra/synccrypto"
	"xquakshell/internal/infra/syncstate"
	"xquakshell/internal/infra/vault"
	presentation "xquakshell/internal/presentation/wails"
	"xquakshell/internal/usecase"
)

// wireSync assembles state sync (ADR-023) and hands it to the Wails API.
//
// The engine writes through the bare vault repository it is given here; the repository's change
// observer is what tells the controller a local edit wants pushing. A failure to open the state
// database leaves sync unavailable rather than the application unable to start: the vault and
// everything in it work the same without it.
func wireSync(api *presentation.AppAPI, vaultRepo *persistence.VaultRepo, dataRoot, vaultDir string) {
	dir := filepath.Join(dataRoot, "sync")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		log.Printf("WARNING: sync unavailable, cannot create %s: %v", dir, err)
		return
	}
	// The composition root owns the state database for the life of the process.
	state, err := syncstate.Open(context.Background(), filepath.Join(dir, "state.db"))
	if err != nil {
		log.Printf("WARNING: sync unavailable, state database: %v", err)
		return
	}
	now := func() int64 { return time.Now().UnixMilli() }
	newEngine := func(s usecase.SyncSession) *usecase.SyncEngine {
		return usecase.NewSyncEngine(usecase.SyncEngineDeps{Server: s.Server, State: state, Vault: vaultRepo, Keys: s.Keys,
			Cipher: synccrypto.Cipher{}, Crypto: synccrypto.Account{}, NowUnixMilli: now})
	}
	accounts := usecase.NewSyncAccountService(usecase.SyncAccountDeps{
		Vault: vaultRepo,
		NewServer: func(url string, pin []byte) (synctrust.AccountServer, error) {
			return syncclient.New(syncclient.Config{BaseURL: url, PinnedIdentity: pin})
		},
		NewAKE:       func(helloContext []byte) (vaultsync.PasswordAKEClient, error) { return opaque.NewClient(helloContext) },
		Crypto:       synccrypto.Account{},
		Random:       rand.Reader,
		NowUnixMilli: now,
		DeviceName:   deviceName(),
		Platform:     runtime.GOOS,
		NewEngine:    newEngine,
		Backup:       vault.Backups{Dir: vaultDir, Now: time.Now},
	})
	controller := usecase.NewSyncController(usecase.SyncControllerDeps{Accounts: accounts, Vault: vaultRepo, State: state,
		NewEngine: newEngine, After: time.After, OnStatus: api.EmitSyncStatus, Random: rand.Reader})
	vaultRepo.SetChangeObserver(controller.Notify)
	api.SetSyncServices(controller, usecase.NewSyncConflictService(vaultRepo))
}

// deviceName labels this device in the account's device list, which every other device of the
// account shows in its new-device alert.
func deviceName() string {
	if h, err := os.Hostname(); err == nil && h != "" {
		return h
	}
	return "xQuakShell device"
}
