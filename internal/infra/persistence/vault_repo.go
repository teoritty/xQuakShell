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
	mu       sync.RWMutex
	dir      string
	session  *vault.Session
	data     *domain.VaultData
	unlocked bool

	// converted records that this unlock upgraded a pre-envelope vault. It is what tells the caller
	// to issue a first recovery key and show it, and it is deliberately not the same question as
	// "has no recovery key": a user who dismissed the one-time dialog by killing the application
	// also has no key, and must ask for a new one rather than be handed one every launch.
	converted bool

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

	session, err := vault.CreateSession(r.dir, masterPassword)
	if err != nil {
		r.data = nil
		return err
	}
	if err := session.Save(snapshot); err != nil {
		r.data = nil
		return err
	}

	// Same ~256 MiB transient scrypt allocation as Unlock —
	// see the scryptWorkFactor comment in internal/infra/vault/vault.go.
	safego.GoNamed("vault.createGC", func() {
		runtime.GC()
		debug.FreeOSMemory()
	})

	r.session = session
	r.unlocked = true
	r.dirty = false
	r.generation = 0
	r.converted = false

	return nil
}

// Unlock decrypts the vault with the given master password.
// It never creates a vault: a missing file yields domain.ErrVaultNotFound, and
// callers must go through Create instead. The minimum-length policy is also
// deliberately not applied here — an existing vault stays openable with
// whatever password created it.
func (r *VaultRepo) Unlock(_ context.Context, masterPassword string) error {
	_, err := r.openWith(masterPassword, false)
	return err
}

// VerifyMasterPassword reports whether masterPassword opens the vault on disk, changing nothing.
//
// It decrypts the file and throws the result away. That costs a full scrypt pass, which is the
// point: an attacker who reached this call gets the same work factor as the unlock screen, and
// there is nothing cheaper to compare against because the master password is never stored.
//
// A recovery key is rejected here even though it opens the vault. Every caller of this is
// re-authenticating for something sensitive - exporting a private key, trusting a plugin - and the
// recovery key is the credential most likely to be sitting on a desk next to the machine.
func (r *VaultRepo) VerifyMasterPassword(_ context.Context, masterPassword string) error {
	r.mu.RLock()
	dir := r.dir
	r.mu.RUnlock()

	if err := vault.VerifyPassword(dir, masterPassword); err != nil {
		return err
	}
	safego.GoNamed("vault.verifyGC", func() {
		runtime.GC()
		debug.FreeOSMemory()
	})
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
	r.session = nil
	r.unlocked = false
	r.dirty = false
	r.converted = false
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
			// Without this the plugin section arrives as its zero value, which reads as "unsigned
			// plugins are fine" - a security default nobody chose. domain.NewVaultData already
			// gets this right; this branch is the other way a Settings struct comes into being.
			Plugins: domain.DefaultPluginSettings(),
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
	session := r.session
	r.mu.Unlock()

	// A flush encrypts the payload to the vault key and runs no key derivation, so unlike Unlock
	// and Create it needs no GC pass afterwards: there is no ~256 MiB scrypt buffer to hand back.
	// This path fires on every connection edit, which is why the credentials are wrapped around a
	// vault key instead of encrypting the file under the password directly.
	err := session.Save(data)

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
