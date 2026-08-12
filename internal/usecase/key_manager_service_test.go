package usecase

import (
	"context"
	"errors"
	"testing"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/keys"
)

// The key manager tests run against the real codec rather than a double. The rules under test are
// about key material - what opens a key, what may leave the vault - and a fake codec would let a
// broken rule pass by answering however the test wanted.
func newKeyManager(t *testing.T, vault domain.VaultRepository) (*KeyManagerService, *memoryIdentityRepo, *recordingCache) {
	t.Helper()
	repo := &memoryIdentityRepo{}
	cache := &recordingCache{values: map[string]string{}}
	svc := NewKeyManagerService(KeyManagerConfig{
		Identities: repo,
		Vault:      vault,
		Codec:      keys.NewCodec(),
		Cache:      cache,
		NewDataKey: keys.NewDataKey,
		Now:        func() time.Time { return time.Unix(1700000000, 0).UTC() },
	})
	return svc, repo, cache
}

type recordingCache struct {
	values  map[string]string
	ttls    map[string]time.Duration
	forgot  []string
	cleared int
}

func (c *recordingCache) Get(id string) (string, bool) { v, ok := c.values[id]; return v, ok }
func (c *recordingCache) Set(id, passphrase string)    { c.values[id] = passphrase }
func (c *recordingCache) SetWithTTL(id, passphrase string, ttl time.Duration) {
	if ttl <= 0 {
		delete(c.values, id)
		return
	}
	if c.ttls == nil {
		c.ttls = map[string]time.Duration{}
	}
	c.values[id] = passphrase
	c.ttls[id] = ttl
}
func (c *recordingCache) Forget(id string) {
	c.forgot = append(c.forgot, id)
	delete(c.values, id)
}
func (c *recordingCache) Clear() { c.cleared++; c.values = map[string]string{} }

type keyTestVault struct {
	data *domain.VaultData
}

func (s keyTestVault) Exists() bool                         { return true }
func (s keyTestVault) Create(context.Context, string) error { return nil }
func (s keyTestVault) Unlock(context.Context, string) error { return nil }
func (s keyTestVault) Lock()                                {}
func (s keyTestVault) IsUnlocked() bool                     { return true }
func (s keyTestVault) GetData() (*domain.VaultData, error)  { return s.data, nil }
func (s keyTestVault) UpdateData(context.Context, func(*domain.VaultData) error) error {
	return nil
}

func ed25519Spec(comment string) domain.GeneratedKeySpec {
	return domain.GeneratedKeySpec{Algorithm: domain.AlgorithmEd25519, Comment: comment}
}

func TestGeneratedVaultPolicyKeyOpensWithoutAskingTheUser(t *testing.T) {
	svc, _, _ := newKeyManager(t, nil)
	ctx := context.Background()

	identity, err := svc.Generate(ctx, ed25519Spec("prod"), "", KeyOptions{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if identity.Policy != domain.KeyPolicyVault {
		t.Errorf("policy = %q, want %q", identity.Policy, domain.KeyPolicyVault)
	}
	if identity.Encrypted {
		t.Error("a vault-policy key is marked as needing a passphrase; the UI would prompt for one that does not exist")
	}
	if _, err := svc.Signer(ctx, identity.ID); err != nil {
		t.Errorf("Signer for a vault-policy key: %v; unlocking the vault must be enough", err)
	}
}

func TestGeneratedPassphraseKeyStaysShutWhileTheVaultIsOpen(t *testing.T) {
	svc, repo, cache := newKeyManager(t, nil)
	ctx := context.Background()

	identity, err := svc.Generate(ctx, ed25519Spec("prod"), "hunter2", KeyOptions{Cache: domain.CacheNever})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if len(repo.blobs[identity.ID].DataKey) != 0 {
		t.Fatal("a data key was stored for a passphrase-policy key; the vault would then hold something that opens it without the user")
	}

	if _, err := svc.Signer(ctx, identity.ID); !errors.Is(err, domain.ErrPassphraseRequired) {
		t.Errorf("Signer with nothing cached = %v, want ErrPassphraseRequired", err)
	}
	if _, err := svc.SignerWithPassphrase(ctx, identity.ID, "wrong"); !errors.Is(err, domain.ErrKeyPassphraseWrong) {
		t.Errorf("SignerWithPassphrase with the wrong passphrase = %v, want ErrKeyPassphraseWrong", err)
	}
	if _, err := svc.SignerWithPassphrase(ctx, identity.ID, "hunter2"); err != nil {
		t.Fatalf("SignerWithPassphrase with the right passphrase: %v", err)
	}
	if _, cached := cache.values[identity.ID]; cached {
		t.Error("a never-cache key had its passphrase cached; the policy is the only thing the user set and it was ignored")
	}
}

func TestCachePolicyDecidesHowLongThePassphraseLives(t *testing.T) {
	svc, _, cache := newKeyManager(t, nil)
	ctx := context.Background()

	bounded, err := svc.Generate(ctx, ed25519Spec("bounded"), "pw", KeyOptions{Cache: domain.CacheDuration, CacheTTLSeconds: 60})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := svc.SignerWithPassphrase(ctx, bounded.ID, "pw"); err != nil {
		t.Fatalf("signer: %v", err)
	}
	if got := cache.ttls[bounded.ID]; got != time.Minute {
		t.Errorf("cached TTL = %v, want 1m; the per-key duration is what the user configured", got)
	}

	untilLock, err := svc.Generate(ctx, ed25519Spec("until-lock"), "pw", KeyOptions{Cache: domain.CacheUntilLock})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := svc.SignerWithPassphrase(ctx, untilLock.ID, "pw"); err != nil {
		t.Fatalf("signer: %v", err)
	}
	if _, bounded := cache.ttls[untilLock.ID]; bounded {
		t.Error("an until-lock key was cached with an expiry; it must live until the vault locks")
	}
}

func TestExportRefusesWithoutReAuthenticationAndWhenNonExportable(t *testing.T) {
	svc, _, _ := newKeyManager(t, nil)
	ctx := context.Background()

	normal, err := svc.Generate(ctx, ed25519Spec("normal"), "", KeyOptions{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := svc.Export(ctx, normal.ID, "", "", false); !errors.Is(err, domain.ErrVaultLocked) {
		t.Errorf("export without re-authentication = %v, want a refusal; the master-password check is the only thing standing between a stray click and a key on disk", err)
	}
	if _, err := svc.Export(ctx, normal.ID, "", "", true); err != nil {
		t.Errorf("export after re-authentication: %v", err)
	}

	sealed, err := svc.Generate(ctx, ed25519Spec("sealed"), "", KeyOptions{NonExportable: true})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := svc.Export(ctx, sealed.ID, "", "", true); !errors.Is(err, domain.ErrKeyNotExportable) {
		t.Errorf("export of a non-exportable key = %v, want ErrKeyNotExportable", err)
	}
}

func TestNonExportableCannotBeUnset(t *testing.T) {
	svc, _, _ := newKeyManager(t, nil)
	ctx := context.Background()
	sealed, err := svc.Generate(ctx, ed25519Spec("sealed"), "", KeyOptions{NonExportable: true})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if err := svc.SetPolicy(ctx, sealed.ID, KeyOptions{NonExportable: false}); !errors.Is(err, domain.ErrKeyNotExportable) {
		t.Fatalf("clearing non-exportable = %v, want a refusal; a promise that a key can never leave must not be undoable with a checkbox", err)
	}
	if _, err := svc.Export(ctx, sealed.ID, "", "", true); !errors.Is(err, domain.ErrKeyNotExportable) {
		t.Errorf("the key became exportable anyway: %v", err)
	}
}

func TestChangePassphraseRewrapsAndForgetsTheOldOne(t *testing.T) {
	svc, _, cache := newKeyManager(t, nil)
	ctx := context.Background()
	identity, err := svc.Generate(ctx, ed25519Spec("key"), "old", KeyOptions{Cache: domain.CacheUntilLock})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := svc.SignerWithPassphrase(ctx, identity.ID, "old"); err != nil {
		t.Fatalf("signer: %v", err)
	}

	if err := svc.ChangePassphrase(ctx, identity.ID, "old", "new"); err != nil {
		t.Fatalf("change passphrase: %v", err)
	}
	if len(cache.forgot) == 0 {
		t.Error("the old passphrase was never forgotten; the next connection would try it, fail, and tell the user their passphrase is wrong right after they changed it")
	}
	if _, err := svc.SignerWithPassphrase(ctx, identity.ID, "old"); !errors.Is(err, domain.ErrKeyPassphraseWrong) {
		t.Errorf("the old passphrase still opens the key = %v; the change did not take", err)
	}
	if _, err := svc.SignerWithPassphrase(ctx, identity.ID, "new"); err != nil {
		t.Errorf("the new passphrase does not open the key: %v", err)
	}
}

func TestRemovingAPassphrasePutsTheKeyUnderTheVault(t *testing.T) {
	svc, repo, _ := newKeyManager(t, nil)
	ctx := context.Background()
	identity, err := svc.Generate(ctx, ed25519Spec("key"), "old", KeyOptions{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	if err := svc.ChangePassphrase(ctx, identity.ID, "old", ""); err != nil {
		t.Fatalf("clear passphrase: %v", err)
	}
	updated, err := repo.Get(ctx, identity.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if updated.Policy != domain.KeyPolicyVault {
		t.Errorf("policy = %q, want %q", updated.Policy, domain.KeyPolicyVault)
	}
	if len(repo.blobs[identity.ID].DataKey) == 0 {
		t.Fatal("no data key after removing the passphrase; the key would be unopenable by anyone")
	}
	if _, err := svc.Signer(ctx, identity.ID); err != nil {
		t.Errorf("Signer after removing the passphrase: %v", err)
	}
}

func TestUsagesFindsBothConnectionUsersAndJumpHops(t *testing.T) {
	data := domain.NewVaultData()
	data.Connections = []domain.Connection{
		{
			ID: "c1", Name: "web",
			Users: []domain.ConnectionUser{{
				ID: "u1", Username: "root", Auth: domain.AuthMethodKey,
				KeyAuth: &domain.KeyAuthConfig{IdentityIDs: []string{"k1"}},
			}},
		},
		{
			ID: "c2", Name: "db",
			JumpChain: domain.JumpChainConfig{Hops: []domain.JumpHop{{
				ID: "h1", Host: "bastion", Username: "jump", Auth: domain.AuthMethodKey,
				KeyAuth: &domain.KeyAuthConfig{IdentityIDs: []string{"k1"}},
			}}},
		},
		{ID: "c3", Name: "unrelated"},
	}
	svc, _, _ := newKeyManager(t, keyTestVault{data: data})

	usages, err := svc.Usages(context.Background(), "k1")
	if err != nil {
		t.Fatalf("usages: %v", err)
	}
	if len(usages) != 2 {
		t.Fatalf("usages = %+v, want 2; a key referenced only by a jump hop is just as much in use, and missing it lets the hop break at connect time", usages)
	}
	var sawHop bool
	for _, u := range usages {
		if u.Hop == "bastion" {
			sawHop = true
		}
	}
	if !sawHop {
		t.Error("the jump-chain reference was not reported")
	}
}

func TestDeleteRefusesWhileAConnectionStillUsesTheKey(t *testing.T) {
	data := domain.NewVaultData()
	data.Connections = []domain.Connection{{
		ID: "c1", Name: "web",
		Users: []domain.ConnectionUser{{
			ID: "u1", Username: "root", Auth: domain.AuthMethodKey,
			KeyAuth: &domain.KeyAuthConfig{IdentityIDs: []string{"in-use"}},
		}},
	}}
	svc, repo, _ := newKeyManager(t, keyTestVault{data: data})
	ctx := context.Background()

	if err := repo.Save(ctx, domain.SSHIdentity{ID: "in-use", Comment: "used"}, domain.IdentityBlob{}); err != nil {
		t.Fatalf("seed: %v", err)
	}
	if err := svc.Delete(ctx, "in-use"); !errors.Is(err, domain.ErrIdentityInUse) {
		t.Fatalf("delete a key in use = %v, want ErrIdentityInUse", err)
	}
	if len(repo.deleted) != 0 {
		t.Error("the key was deleted despite the refusal; the connection would fail only at connect time, far from the action that broke it")
	}
}

func TestDeleteRemovesAnUnusedKeyAndItsCachedPassphrase(t *testing.T) {
	svc, repo, cache := newKeyManager(t, keyTestVault{data: domain.NewVaultData()})
	ctx := context.Background()
	identity, err := svc.Generate(ctx, ed25519Spec("spare"), "pw", KeyOptions{})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if _, err := svc.SignerWithPassphrase(ctx, identity.ID, "pw"); err != nil {
		t.Fatalf("signer: %v", err)
	}

	if err := svc.Delete(ctx, identity.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if len(repo.deleted) != 1 {
		t.Errorf("deleted = %v, want the key removed", repo.deleted)
	}
	if _, still := cache.values[identity.ID]; still {
		t.Error("the deleted key's passphrase is still cached; nothing will ever clear it before the vault locks")
	}
}

func TestAKeyMustBeNamed(t *testing.T) {
	svc, _, _ := newKeyManager(t, nil)
	ctx := context.Background()
	if _, err := svc.Generate(ctx, ed25519Spec("  "), "", KeyOptions{}); !errors.Is(err, domain.ErrIdentityNameRequired) {
		t.Errorf("generate with a blank label = %v, want ErrIdentityNameRequired; an unnamed key is one the user cannot tell apart from another", err)
	}
	if err := svc.Rename(ctx, "any", ""); !errors.Is(err, domain.ErrIdentityNameRequired) {
		t.Errorf("rename to blank = %v, want ErrIdentityNameRequired", err)
	}
}

func TestPublicKeyIsServedWithoutUnwrappingAnything(t *testing.T) {
	svc, _, _ := newKeyManager(t, nil)
	ctx := context.Background()
	identity, err := svc.Generate(ctx, ed25519Spec("key"), "never-supplied", KeyOptions{Cache: domain.CacheNever})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}

	// Nothing has the passphrase, so a public key derived on demand would be impossible to serve.
	pub, err := svc.PublicKey(ctx, identity.ID)
	if err != nil {
		t.Fatalf("public key: %v", err)
	}
	if pub != identity.PublicKey || pub == "" {
		t.Errorf("public key = %q, want the stored line %q", pub, identity.PublicKey)
	}
}
