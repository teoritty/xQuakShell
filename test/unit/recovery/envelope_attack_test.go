package recovery

import (
	"context"
	"encoding/base64"
	"errors"
	"os"
	"strings"
	"testing"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/persistence"
	"xquakshell/internal/infra/vault"
)

// Both credentials open the same data. If they did not, one of them would be opening something
// else, which is the failure mode a second credential exists to avoid.
func TestBothCredentialsOpenTheSameVault(t *testing.T) {
	f := newFixture(t)
	if err := f.repo.UpdateData(context.Background(), func(d *domain.VaultData) error {
		d.KnownHosts = []string{"marker.example ssh-ed25519 AAAA"}
		return nil
	}); err != nil {
		t.Fatalf("write marker: %v", err)
	}

	for _, tc := range []struct {
		name       string
		credential string
		want       domain.UnlockMethod
	}{
		{"password", f.password, domain.UnlockByPassword},
		{"recovery key", domain.FormatRecoveryKey(f.key), domain.UnlockByRecoveryKey},
	} {
		repo := f.reopen(t)
		method, err := repo.UnlockWithCredential(context.Background(), tc.credential)
		if err != nil {
			t.Fatalf("%s: unlock: %v", tc.name, err)
		}
		if method != tc.want {
			t.Errorf("%s: method = %v, want %v", tc.name, method, tc.want)
		}
		data, err := repo.GetData()
		if err != nil {
			t.Fatalf("%s: get data: %v", tc.name, err)
		}
		if len(data.KnownHosts) != 1 || data.KnownHosts[0] != "marker.example ssh-ed25519 AAAA" {
			t.Errorf("%s: opened a vault without the marker; the two credentials are not reaching the same data", tc.name)
		}
		f.repo = repo
	}
}

// Removing a wrap from the file must revoke exactly that credential and leave the other working.
// A design where deleting one wrap also broke the other would mean the wraps are not independent,
// and a corrupted byte in one would cost the user everything.
func TestDeletingOneWrapLeavesTheOtherWorking(t *testing.T) {
	f := newFixture(t)
	f.repo.Lock()

	env := rawEnvelope(t, f.dir)
	dropWrap(t, env, "recovery")
	writeEnvelope(t, f.dir, env)

	repo := persistence.NewVaultRepo(f.dir)
	if _, err := repo.UnlockWithCredential(context.Background(), domain.FormatRecoveryKey(f.key)); !errors.Is(err, domain.ErrVaultDecryptFailed) {
		t.Errorf("recovery key after its wrap was deleted: got %v, want ErrVaultDecryptFailed", err)
	}
	if _, err := repo.UnlockWithCredential(context.Background(), f.password); err != nil {
		t.Errorf("password after the recovery wrap was deleted: %v; the two wraps must be independent", err)
	}
}

// A wrap taken from another vault wraps another vault's key. Splicing one in must fail rather than
// produce a session pointing at a key that decrypts nothing - or, worse, one that is accepted and
// then used to re-encrypt this vault's data.
func TestAWrapFromAnotherVaultDoesNotOpenThisOne(t *testing.T) {
	victim := newFixture(t)
	attacker := newFixture(t)
	victim.repo.Lock()
	attacker.repo.Lock()

	donor := rawEnvelope(t, attacker.dir)
	target := rawEnvelope(t, victim.dir)

	var donorRecovery any
	for _, w := range wraps(t, donor) {
		if entry, ok := w.(map[string]any); ok && entry["kind"] == "recovery" {
			donorRecovery = w
		}
	}
	if donorRecovery == nil {
		t.Fatal("the donor vault has no recovery wrap to splice")
	}
	dropWrap(t, target, "recovery")
	target["wraps"] = append(wraps(t, target), donorRecovery)
	writeEnvelope(t, victim.dir, target)

	repo := persistence.NewVaultRepo(victim.dir)
	if _, err := repo.UnlockWithCredential(context.Background(), domain.FormatRecoveryKey(attacker.key)); err == nil {
		t.Fatal("a recovery key from a different vault opened this one; the wrap is not bound to the payload it sits beside")
	}
	if repo.IsUnlocked() {
		t.Error("the repository reports unlocked after a spliced wrap was refused; every write path keys on this flag")
	}
}

// Every part of the file is authenticated. A flipped bit must surface as a refusal, never as
// silently different data - age's AEAD is what provides this, and this test is what notices if the
// payload ever stops going through it.
func TestATamperedFileIsRefusedRatherThanSilentlyAccepted(t *testing.T) {
	cases := []string{"payload", "wrap"}

	for _, part := range cases {
		f := newFixture(t)
		f.repo.Lock()
		env := rawEnvelope(t, f.dir)

		if part == "payload" {
			env["payload"] = flipFirstByte(t, env["payload"])
		} else {
			list := wraps(t, env)
			entry, ok := list[0].(map[string]any)
			if !ok {
				t.Fatalf("unexpected wrap shape: %#v", list[0])
			}
			entry["data"] = flipFirstByte(t, entry["data"])
		}
		writeEnvelope(t, f.dir, env)

		repo := persistence.NewVaultRepo(f.dir)
		_, err := repo.UnlockWithCredential(context.Background(), f.password)
		if err == nil {
			t.Errorf("a tampered %s opened anyway", part)
			continue
		}
		if repo.IsUnlocked() {
			t.Errorf("a tampered %s left the repository unlocked", part)
		}
	}
}

// A build must not read a newer envelope, and must not overwrite one either. Both directions are
// separate failures because only one of them destroys data, and the message has to name which
// action the user should take.
func TestANewerEnvelopeIsNeitherReadNorOverwritten(t *testing.T) {
	f := newFixture(t)
	f.repo.Lock()

	env := rawEnvelope(t, f.dir)
	env["envelope"] = float64(vault.CurrentEnvelopeVersion + 1)
	writeEnvelope(t, f.dir, env)

	before, err := os.ReadFile(vault.FilePath(f.dir))
	if err != nil {
		t.Fatalf("read vault: %v", err)
	}

	repo := persistence.NewVaultRepo(f.dir)
	if _, err := repo.UnlockWithCredential(context.Background(), f.password); !errors.Is(err, domain.ErrVaultEnvelopeTooNew) {
		t.Fatalf("unlock a newer envelope: got %v, want ErrVaultEnvelopeTooNew", err)
	}
	if repo.IsUnlocked() {
		t.Error("the repository reports unlocked after refusing a newer envelope")
	}
	if err := repo.Create(context.Background(), f.password); !errors.Is(err, domain.ErrVaultAlreadyExists) {
		t.Fatalf("create over a newer envelope: got %v, want ErrVaultAlreadyExists", err)
	}

	// Lock runs the flush path, which is where an unguarded write would land.
	repo.Lock()

	after, err := os.ReadFile(vault.FilePath(f.dir))
	if err != nil {
		t.Fatalf("read vault after: %v", err)
	}
	if string(before) != string(after) {
		t.Error("the vault file changed after a refused unlock; a rollback to this build would now be losing whatever the newer build stored")
	}
}

// The unlock throttle only matters for someone at the running application. Someone holding the file
// attacks it offline, so the wraps have to be the expensive part - an envelope that stored the key
// unwrapped, or wrapped at a lower work factor, would look identical from the outside.
func TestTheVaultKeyNeverAppearsOutsideAWrap(t *testing.T) {
	f := newFixture(t)
	f.repo.Lock()

	raw, err := os.ReadFile(vault.FilePath(f.dir))
	if err != nil {
		t.Fatalf("read vault: %v", err)
	}
	text := string(raw)

	// An age X25519 private key is textual and starts with this prefix. Finding it in the file at
	// all would mean the vault key is sitting there for anyone who opens the file in an editor.
	if strings.Contains(text, "AGE-SECRET-KEY-") {
		t.Fatal("the vault key is stored in the clear; the wraps are decoration")
	}
	if strings.Contains(text, f.key) || strings.Contains(text, domain.FormatRecoveryKey(f.key)) {
		t.Fatal("the recovery key itself is in the vault file")
	}
	if strings.Contains(text, f.password) {
		t.Fatal("the master password is in the vault file")
	}
}

func flipFirstByte(t *testing.T, encoded any) string {
	t.Helper()
	text, ok := encoded.(string)
	if !ok {
		t.Fatalf("expected a base64 string, got %#v", encoded)
	}
	raw, err := base64.StdEncoding.DecodeString(text)
	if err != nil {
		t.Fatalf("decode base64: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("nothing to tamper with")
	}
	raw[0] ^= 0xff
	return base64.StdEncoding.EncodeToString(raw)
}
