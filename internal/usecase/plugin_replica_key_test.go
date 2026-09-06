package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"xquakshell/internal/domain"
)

// replicaKeyVault is the smallest vault a key can live in, plus a switch for the case that matters
// most: a mint whose write does not land.
type replicaKeyVault struct {
	data      *domain.VaultData
	updateErr error
}

func (v *replicaKeyVault) Exists() bool                                       { return true }
func (v *replicaKeyVault) Create(context.Context, string) error               { return nil }
func (v *replicaKeyVault) Unlock(context.Context, string) error               { return nil }
func (v *replicaKeyVault) VerifyMasterPassword(context.Context, string) error { return nil }
func (v *replicaKeyVault) Lock()                                              {}
func (v *replicaKeyVault) IsUnlocked() bool                                   { return true }
func (v *replicaKeyVault) GetData() (*domain.VaultData, error)                { return v.data, nil }
func (v *replicaKeyVault) UpdateData(_ context.Context, mutate func(*domain.VaultData) error) error {
	if v.updateErr != nil {
		return v.updateErr
	}
	return mutate(v.data)
}

func keyService(t *testing.T) (*PluginVaultSettings, *replicaKeyVault) {
	t.Helper()
	vault := &replicaKeyVault{data: domain.NewVaultData()}
	return NewPluginVaultSettings(vault), vault
}

// Minting produces a key the user can be shown and, on the other device, type back in.
func TestAMintedKeyIsInTheFormatAPersonCanCopy(t *testing.T) {
	service, _ := keyService(t)

	key, err := service.MintReplicaKey(context.Background(), "com.example.sync")
	if err != nil {
		t.Fatalf("MintReplicaKey: %v", err)
	}

	if len(key) != domain.RecoveryKeyLength {
		t.Fatalf("key %q is %d characters, want %d", key, len(key), domain.RecoveryKeyLength)
	}
	if normalized, ok := domain.NormalizeRecoveryKey(key); !ok || normalized != key {
		t.Fatalf("a minted key does not survive normalisation: %q", key)
	}
	if strings.ContainsAny(key, "ILO") {
		t.Errorf("key %q contains a character that reads as a digit", key)
	}
}

// Two mints must not agree. A key derived from the plugin id, or from anything else stable, would
// be the same on every installation - which is the same as no key at all.
func TestTwoMintedKeysDiffer(t *testing.T) {
	service, _ := keyService(t)

	first, err := service.MintReplicaKey(context.Background(), "com.example.sync")
	if err != nil {
		t.Fatalf("MintReplicaKey: %v", err)
	}
	second, err := service.MintReplicaKey(context.Background(), "com.example.other")
	if err != nil {
		t.Fatalf("MintReplicaKey: %v", err)
	}

	if first == second {
		t.Fatal("two mints produced the same key")
	}
}

// The key is what opens every replica this plugin ever sealed. Minting over it silently would leave
// the user with a device that can no longer read its own history and no way to discover why.
func TestMintingOverAnExistingKeyIsRefused(t *testing.T) {
	service, store := keyService(t)
	first, err := service.MintReplicaKey(context.Background(), "com.example.sync")
	if err != nil {
		t.Fatalf("MintReplicaKey: %v", err)
	}

	_, err = service.MintReplicaKey(context.Background(), "com.example.sync")

	if !errors.Is(err, domain.ErrReplicaKeyExists) {
		t.Fatalf("err = %v, want ErrReplicaKeyExists", err)
	}
	if got := store.data.ReplicaKeys["com.example.sync"]; got != first {
		t.Fatalf("the stored key changed to %q", got)
	}
}

// The second device types in the key from the first. Everything a person adds while copying from
// paper - lowercase, the display dashes, a trailing newline from a paste - has to be accepted, or
// the feature fails for the ordinary user doing the ordinary thing.
func TestATypedKeyIsAcceptedHoweverItWasCopied(t *testing.T) {
	service, store := keyService(t)
	minted, err := service.MintReplicaKey(context.Background(), "com.example.sync")
	if err != nil {
		t.Fatalf("MintReplicaKey: %v", err)
	}

	for _, typed := range []string{
		domain.FormatRecoveryKey(minted),
		strings.ToLower(domain.FormatRecoveryKey(minted)),
		" " + minted + "\n",
	} {
		store.data.ReplicaKeys = nil
		if err := service.ImportReplicaKey(context.Background(), "com.example.sync", typed); err != nil {
			t.Fatalf("%q: ImportReplicaKey: %v", typed, err)
		}
		if got := store.data.ReplicaKeys["com.example.sync"]; got != minted {
			t.Errorf("%q stored as %q, want the canonical %q", typed, got, minted)
		}
	}
}

// A mistyped key must be refused at the point of entry. Storing it would produce a device that
// silently fails every fetch with "cannot be opened", which reads as data loss rather than a typo.
func TestAMalformedKeyIsRefused(t *testing.T) {
	service, store := keyService(t)

	for _, typed := range []string{"", "too-short", strings.Repeat("A", domain.RecoveryKeyLength+1), "!!!!"} {
		if err := service.ImportReplicaKey(context.Background(), "com.example.sync", typed); !errors.Is(err, domain.ErrReplicaKeyMalformed) {
			t.Errorf("%q: err = %v, want ErrReplicaKeyMalformed", typed, err)
		}
	}
	if len(store.data.ReplicaKeys) != 0 {
		t.Fatalf("a refused import stored %d keys", len(store.data.ReplicaKeys))
	}
}

// Importing is how the second device joins, so it does replace what is there - unlike minting. The
// user asked for this one explicitly, key in hand.
func TestImportingReplacesAnExistingKey(t *testing.T) {
	service, store := keyService(t)
	if _, err := service.MintReplicaKey(context.Background(), "com.example.sync"); err != nil {
		t.Fatalf("MintReplicaKey: %v", err)
	}
	other := strings.Repeat("A", domain.RecoveryKeyLength)

	if err := service.ImportReplicaKey(context.Background(), "com.example.sync", other); err != nil {
		t.Fatalf("ImportReplicaKey: %v", err)
	}

	if got := store.data.ReplicaKeys["com.example.sync"]; got != other {
		t.Fatalf("stored key = %q, want the imported one", got)
	}
}

// A plugin with no key has not been set up for replication yet. That is a state the UI reports, so
// it has to be distinguishable from a failure to read the vault.
func TestAPluginWithNoKeyReportsThatRatherThanFailing(t *testing.T) {
	service, _ := keyService(t)

	key, err := service.ReplicaKey(context.Background(), "com.example.sync")

	if !errors.Is(err, domain.ErrReplicaKeyRequired) {
		t.Fatalf("err = %v, want ErrReplicaKeyRequired", err)
	}
	if key != "" {
		t.Errorf("key = %q, want nothing alongside the error", key)
	}
}

// Each plugin's key opens only its own replicas. One shared key would mean a hostile plugin that
// ever got hold of a second plugin's ciphertext could read it.
func TestEachPluginGetsItsOwnKey(t *testing.T) {
	service, _ := keyService(t)
	if _, err := service.MintReplicaKey(context.Background(), "com.example.sync"); err != nil {
		t.Fatalf("MintReplicaKey: %v", err)
	}

	if _, err := service.ReplicaKey(context.Background(), "com.example.other"); !errors.Is(err, domain.ErrReplicaKeyRequired) {
		t.Fatalf("err = %v; another plugin's key was visible", err)
	}
}

// Reading back what was minted is the whole point: seal on this device, open on the next.
func TestTheKeyReadsBackAsItWasMinted(t *testing.T) {
	service, _ := keyService(t)
	minted, err := service.MintReplicaKey(context.Background(), "com.example.sync")
	if err != nil {
		t.Fatalf("MintReplicaKey: %v", err)
	}

	got, err := service.ReplicaKey(context.Background(), "com.example.sync")
	if err != nil {
		t.Fatalf("ReplicaKey: %v", err)
	}

	if got != minted {
		t.Fatalf("read back %q, minted %q", got, minted)
	}
}

// Uninstalling has to take the key with it, or reinstalling the same plugin silently reconnects to
// a history the user believed they had disconnected from - and the replicas on the server, which
// they cannot delete from here, become readable again.
func TestUninstallingAPluginRemovesItsReplicationKey(t *testing.T) {
	service, store := keyService(t)
	if _, err := service.MintReplicaKey(context.Background(), "com.example.sync"); err != nil {
		t.Fatalf("MintReplicaKey: %v", err)
	}
	if _, err := service.MintReplicaKey(context.Background(), "com.example.other"); err != nil {
		t.Fatalf("MintReplicaKey: %v", err)
	}

	if err := service.RevokeAllGrants(context.Background(), "com.example.sync"); err != nil {
		t.Fatalf("RevokeAllGrants: %v", err)
	}

	if _, present := store.data.ReplicaKeys["com.example.sync"]; present {
		t.Error("the replication key survived the plugin being uninstalled")
	}
	if _, present := store.data.ReplicaKeys["com.example.other"]; !present {
		t.Error("uninstalling one plugin took another plugin's key with it")
	}
}

// A vault that cannot be written must not report a key the user could write down: they would copy
// it to the second device and find the first has no record of it.
func TestAKeyIsNotReportedWhenItCouldNotBeStored(t *testing.T) {
	vault := &replicaKeyVault{data: domain.NewVaultData(), updateErr: errors.New("vault is read-only")}
	service := NewPluginVaultSettings(vault)

	key, err := service.MintReplicaKey(context.Background(), "com.example.sync")

	if err == nil {
		t.Fatal("a key was minted into a vault that cannot be written")
	}
	if key != "" {
		t.Errorf("key = %q, want nothing alongside the error", key)
	}
}
