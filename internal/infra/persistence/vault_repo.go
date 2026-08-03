package persistence

import (
	"context"
	"log/slog"
	"runtime"
	"runtime/debug"
	"sync"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/vault"
	"xquakshell/internal/pkg/safego"
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

// Exists reports whether a vault file is present on disk.
//
// This is advisory only: the answer can change between the call and any
// follow-up action. The authoritative existence check lives inside Create,
// where it runs under the same write lock as the write itself.
func (r *VaultRepo) Exists() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return vault.Exists(r.dir)
}

// Create writes a brand-new empty vault encrypted with masterPassword and
// leaves the repository unlocked, so creating a master password immediately
// opens the app.
//
// It returns domain.ErrMasterPasswordTooShort for a password below
// domain.MinMasterPasswordLength and domain.ErrVaultAlreadyExists rather than
// overwriting an existing vault.
//
// The write is synchronous and deliberately bypasses the debounced flush path
// used by UpdateData. A debounced create would leave a vaultPersistDebounce-wide
// window in which the user believes a master password is set while nothing is on
// disk yet, and flushGeneration only logs write failures where the caller needs
// a real error.
//
// The existence check and the write are covered by a single r.mu write lock, so
// two concurrent Creates serialize and the loser sees the winner's file. That
// guarantee is process-local: WriteVaultFile writes a temp file and renames, so
// it cannot use O_EXCL on the final path, and a second xQuakShell process
// pointed at the same vault directory could still race. Accepted deliberately
// for a single-instance desktop app; a cross-process guard would need a lock
// file.
func (r *VaultRepo) Create(_ context.Context, masterPassword string) error {
	// Validate before taking the lock so a rejected password never touches the
	// mutex or the disk.
	if len([]rune(masterPassword)) < domain.MinMasterPasswordLength {
		return domain.ErrMasterPasswordTooShort
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if vault.Exists(r.dir) {
		return domain.ErrVaultAlreadyExists
	}

	r.data = domain.NewVaultData()
	r.ensureVaultDataLocked()
	snapshot := domain.CloneVaultData(r.data)

	if err := vault.WriteVaultFile(r.dir, masterPassword, snapshot); err != nil {
		r.data = nil
		return err
	}

	// Same ~256 MiB transient scrypt allocation as Unlock and flushGeneration —
	// see the SetWorkFactor comment in internal/infra/vault/vault.go.
	safego.GoNamed("vault.createGC", func() {
		runtime.GC()
		debug.FreeOSMemory()
	})

	r.passphrase = masterPassword
	r.unlocked = true
	r.dirty = false
	r.generation = 0

	return nil
}

// Unlock decrypts the vault with the given master password.
// It never creates a vault: a missing file yields domain.ErrVaultNotFound, and
// callers must go through Create instead. The minimum-length policy is also
// deliberately not applied here — an existing vault stays openable with
// whatever password created it.
func (r *VaultRepo) Unlock(_ context.Context, masterPassword string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	data, err := vault.ReadVaultFile(r.dir, masterPassword)
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
	safego.GoNamed("vault.unlockGC", func() {
		runtime.GC()
		debug.FreeOSMemory()
	})

	r.passphrase = masterPassword
	r.data = data
	r.ensureVaultDataLocked()
	r.unlocked = true
	r.dirty = false
	r.generation = 0

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

// GetData returns a deep snapshot of the current in-memory vault data.
func (r *VaultRepo) GetData() (*domain.VaultData, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	if !r.unlocked {
		return nil, domain.ErrVaultLocked
	}
	return domain.CloneVaultData(r.data), nil
}

// UpdateData applies a mutation to vault data atomically under the write lock.
func (r *VaultRepo) UpdateData(_ context.Context, mutate func(*domain.VaultData) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if !r.unlocked {
		return domain.ErrVaultLocked
	}
	r.ensureVaultDataLocked()
	if err := mutate(r.data); err != nil {
		return err
	}
	r.dirty = true
	r.generation++
	r.scheduleFlushLocked()
	return nil
}

func (r *VaultRepo) ensureVaultDataLocked() {
	if r.data == nil {
		r.data = domain.NewVaultData()
	}
	if r.data.Identities == nil {
		r.data.Identities = map[string]domain.SSHIdentity{}
	}
	if r.data.KeyBlobs == nil {
		r.data.KeyBlobs = map[string]domain.IdentityBlob{}
	}
	if r.data.Passwords == nil {
		r.data.Passwords = map[string]domain.PasswordBlob{}
	}
	if r.data.PluginSecrets == nil {
		r.data.PluginSecrets = map[string][]byte{}
	}
	if r.data.Settings == nil {
		r.data.Settings = &domain.AppSettings{
			Lockout:  domain.DefaultLockoutSettings(),
			Terminal: domain.DefaultTerminalSettings(),
			Theme:    "dark",
		}
	}
	if r.data.Settings.Terminal.FontFamily == "" {
		r.data.Settings.Terminal = domain.DefaultTerminalSettings()
	}
	if r.data.Settings.Theme == "" {
		r.data.Settings.Theme = "dark"
	}
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
	data := domain.CloneVaultData(r.data)
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
	safego.GoNamed("vault.flushGC", func() {
		runtime.GC()
		debug.FreeOSMemory()
	})

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
