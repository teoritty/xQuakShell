package persistence

import (
	"context"
	"log/slog"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/vault"
)

const vaultPersistDebounce = 400 * time.Millisecond

// VaultRepo implements domain.VaultRepository backed by an age-encrypted file.
type VaultRepo struct {
	mu         sync.RWMutex
	dir        string
	passphrase string
	data       *domain.VaultData
	unlocked   bool

	dirty      bool
	generation uint64
	flushTimer *time.Timer
	flushMu    sync.Mutex
}

// NewVaultRepo creates a new VaultRepo that stores vault.age in the given directory.
func NewVaultRepo(dir string) *VaultRepo {
	return &VaultRepo{dir: dir}
}

// Unlock decrypts the vault with the given master password.
// If the vault file does not exist, a new empty vault is created and saved.
func (r *VaultRepo) Unlock(_ context.Context, masterPassword string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, needsPersist, err := vault.ReadVaultFile(r.dir, masterPassword)
	if err != nil {
		return err
	}

	// ReadVaultFile -> Decrypt runs the same scrypt KDF as Encrypt (see the
	// SetWorkFactor comment in internal/infra/vault/vault.go) and transiently
	// allocates ~256 MiB while doing so. Force the Go runtime to release
	// those pages back to the OS immediately, mirroring the identical
	// workaround already used after vault writes below in flushNow().
	// Without this, unlocking the vault produces an RSS spike that can
	// visibly linger for several minutes before the runtime's background
	// scavenger reclaims it on its own. Runs in a goroutine so it never
	// blocks the caller waiting on Unlock's return.
	go func() {
		runtime.GC()
		debug.FreeOSMemory()
	}()

	if data.Identities == nil {
		data.Identities = map[string]domain.SSHIdentity{}
	}
	if data.KeyBlobs == nil {
		data.KeyBlobs = map[string]domain.IdentityBlob{}
	}
	if data.Passwords == nil {
		data.Passwords = map[string]domain.PasswordBlob{}
	}
	if data.Settings == nil {
		data.Settings = &domain.AppSettings{
			Lockout:  domain.DefaultLockoutSettings(),
			Terminal: domain.DefaultTerminalSettings(),
			Theme:    "dark",
		}
	}
	if data.Settings.Terminal.FontFamily == "" {
		data.Settings.Terminal = domain.DefaultTerminalSettings()
	}
	if data.Settings.Theme == "" {
		data.Settings.Theme = "dark"
	}

	r.passphrase = masterPassword
	r.data = data
	r.unlocked = true
	r.dirty = false
	r.generation = 0

	if needsPersist {
		r.dirty = true
		r.generation = 1
		r.scheduleFlushLocked()
	}

	return nil
}

// Lock flushes pending changes, then clears decrypted data from memory.
func (r *VaultRepo) Lock() {
	r.flushNow()
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.flushTimer != nil {
		r.flushTimer.Stop()
		r.flushTimer = nil
	}
	r.data = nil
	r.passphrase = ""
	r.unlocked = false
	r.dirty = false
}

// IsUnlocked returns true when the vault is decrypted in memory.
func (r *VaultRepo) IsUnlocked() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.unlocked
}

// GetData returns the current in-memory vault data.
func (r *VaultRepo) GetData() (*domain.VaultData, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.unlocked {
		return nil, domain.ErrVaultLocked
	}
	return r.data, nil
}

// SaveData updates in-memory vault data and schedules a debounced encrypted write.
func (r *VaultRepo) SaveData(_ context.Context, data *domain.VaultData) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.unlocked {
		return domain.ErrVaultLocked
	}
	r.data = data
	r.dirty = true
	r.generation++
	r.scheduleFlushLocked()
	return nil
}

func (r *VaultRepo) scheduleFlushLocked() {
	if r.flushTimer != nil {
		r.flushTimer.Stop()
	}
	gen := r.generation
	r.flushTimer = time.AfterFunc(vaultPersistDebounce, func() {
		r.flushGeneration(gen)
	})
}

func (r *VaultRepo) flushNow() {
	r.mu.Lock()
	if r.flushTimer != nil {
		r.flushTimer.Stop()
		r.flushTimer = nil
	}
	gen := r.generation
	dirty := r.dirty
	r.mu.Unlock()
	if dirty {
		r.flushGeneration(gen)
	}
}

func (r *VaultRepo) flushGeneration(gen uint64) {
	r.flushMu.Lock()
	defer r.flushMu.Unlock()

	r.mu.Lock()
	if !r.unlocked || !r.dirty {
		r.mu.Unlock()
		return
	}
	if r.generation != gen {
		r.scheduleFlushLocked()
		r.mu.Unlock()
		return
	}
	data := r.data
	passphrase := r.passphrase
	dir := r.dir
	r.mu.Unlock()

	err := vault.WriteVaultFile(dir, passphrase, data)

	// vault.WriteVaultFile (Encrypt) runs the same scrypt key derivation as
	// vault.ReadVaultFile (Decrypt) — see the SetWorkFactor comment in
	// internal/infra/vault/vault.go for why that transiently costs ~256 MiB.
	// Force an immediate GC pass and release those pages back to the OS here
	// so the RSS spike collapses right after the save completes instead of
	// lingering for minutes while the Go runtime's background scavenger gets
	// around to it on its own schedule. Runs in a goroutine so it never
	// blocks the caller waiting on this flush.
	go func() {
		runtime.GC()
		debug.FreeOSMemory()
	}()

	r.mu.Lock()
	if err != nil {
		slog.Error("vault flush failed", "err", err)
	} else if r.generation == gen {
		r.dirty = false
	} else if r.dirty {
		r.scheduleFlushLocked()
	}
	r.mu.Unlock()
}
