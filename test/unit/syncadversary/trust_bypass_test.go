package syncadversary

import (
	"crypto/ed25519"
	"crypto/hmac"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1" // #nosec G505 -- builds OpenSSH's hashed known_hosts form, which is HMAC-SHA1
	"encoding/base64"
	"errors"
	"strings"
	"testing"

	gossh "golang.org/x/crypto/ssh"

	"xquakshell/internal/domain"
	"xquakshell/internal/infra/persistence"
	"xquakshell/internal/usecase"
)

// The attacker in these tests is B: a device of the same account - a stolen device, or one enrolled
// with a stolen sync password. Whatever B commits, a change that widens what A trusts must wait for
// A's master password, however B spells it.

func edKey(t *testing.T) gossh.PublicKey {
	t.Helper()
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	k, err := gossh.NewPublicKey(pub)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func rsaKey(t *testing.T) gossh.PublicKey {
	t.Helper()
	rk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	k, err := gossh.NewPublicKey(&rk.PublicKey)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func knownHostLine(host string, k gossh.PublicKey) string {
	return host + " " + strings.TrimSpace(string(gossh.MarshalAuthorizedKey(k)))
}

func hashedHost(host string) string {
	salt := make([]byte, 20)
	_, _ = rand.Read(salt)
	m := hmac.New(sha1.New, salt)
	m.Write([]byte(host))
	return "|1|" + base64.StdEncoding.EncodeToString(salt) + "|" + base64.StdEncoding.EncodeToString(m.Sum(nil))
}

// syncRounds has B apply each edit to its vault and sync it, and A sync after each, and returns A's
// pull reports.
func syncRounds(t *testing.T, a, b *device, rounds ...func(*domain.VaultData)) []usecase.PullReport {
	t.Helper()
	var out []usecase.PullReport
	for _, round := range rounds {
		_ = b.vault.UpdateData(ctx, func(v *domain.VaultData) error { round(v); return nil })
		b.sync()
		r, err := a.engine.Sync(ctx, a.session.Personal)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, r.Pull)
	}
	return out
}

func TestAnotherDeviceCannotPlantAHostKeyForAStoredHost(t *testing.T) {
	cases := map[string]func(evil gossh.PublicKey) []func(*domain.VaultData){
		"the stored line replaced": func(evil gossh.PublicKey) []func(*domain.VaultData) {
			return []func(*domain.VaultData){func(v *domain.VaultData) { v.KnownHosts = []string{knownHostLine("web.example", evil)} }}
		},
		"the host in hashed form beside it": func(evil gossh.PublicKey) []func(*domain.VaultData) {
			return []func(*domain.VaultData){func(v *domain.VaultData) {
				v.KnownHosts = append(v.KnownHosts, knownHostLine(hashedHost("web.example"), evil))
			}}
		},
		"a marker in front of the host": func(evil gossh.PublicKey) []func(*domain.VaultData) {
			return []func(*domain.VaultData){func(v *domain.VaultData) {
				v.KnownHosts = append(v.KnownHosts, "@revoked "+knownHostLine("web.example", evil))
			}}
		},
		"deleted, then added back with another key": func(evil gossh.PublicKey) []func(*domain.VaultData) {
			return []func(*domain.VaultData){
				func(v *domain.VaultData) { v.KnownHosts = nil },
				func(v *domain.VaultData) { v.KnownHosts = []string{knownHostLine("web.example", evil)} },
			}
		},
	}
	for name, rounds := range cases {
		t.Run(name, func(t *testing.T) {
			assertHostKeyHeld(t, edKey(t), rounds)
		})
	}
	t.Run("a second key type for the host", func(t *testing.T) {
		assertHostKeyHeld(t, rsaKey(t), func(evil gossh.PublicKey) []func(*domain.VaultData) {
			return []func(*domain.VaultData){func(v *domain.VaultData) {
				v.KnownHosts = append(v.KnownHosts, knownHostLine("web.example", evil))
			}}
		})
	})
}

func assertHostKeyHeld(t *testing.T, evil gossh.PublicKey, rounds func(gossh.PublicKey) []func(*domain.VaultData)) {
	t.Helper()
	_, a, b := pair(t)
	stored := edKey(t)
	_ = a.vault.UpdateData(ctx, func(v *domain.VaultData) error {
		v.KnownHosts = append(v.KnownHosts, knownHostLine("web.example", stored))
		return nil
	})
	a.sync()
	b.sync()
	reports := syncRounds(t, a, b, rounds(evil)...)
	if last := reports[len(reports)-1]; last.Held != 1 || last.Applied != 0 {
		t.Fatalf("A's last pull = %+v; the change must wait for A's master password", last)
	}
	repo := persistence.NewKnownHostsRepo(a.vault)
	if err := repo.Check("web.example", evil); !errors.Is(err, domain.ErrHostKeyMismatch) {
		t.Fatalf("A checking the planted key: %v, want ErrHostKeyMismatch", err)
	}
	if err := repo.Check("web.example", stored); err != nil {
		t.Fatalf("A checking its own key: %v; the stored key must stay in force", err)
	}
}

func TestAnotherDeviceCannotSwapAPeerIdentityByDeletingIt(t *testing.T) {
	_, a, b := pair(t)
	entry := func(m string) domain.PeerTrustEntry {
		return domain.PeerTrustEntry{Scope: "plugin.x", Subject: "conn-1", Material: []byte(m)}
	}
	_ = a.vault.UpdateData(ctx, func(v *domain.VaultData) error { v.PeerTrust = []domain.PeerTrustEntry{entry("good")}; return nil })
	a.sync()
	b.sync()
	reports := syncRounds(t, a, b,
		func(v *domain.VaultData) { v.PeerTrust = nil },
		func(v *domain.VaultData) { v.PeerTrust = []domain.PeerTrustEntry{entry("evil")} })
	for i, r := range reports {
		if r.Held != 1 || r.Applied != 0 {
			t.Fatalf("A's pull %d = %+v; deleting and replacing a trusted peer identity both wait", i, r)
		}
	}
	if got, _ := a.vault.GetData(); len(got.PeerTrust) != 1 || string(got.PeerTrust[0].Material) != "good" {
		t.Fatalf("A's peer trust = %+v; the identity A trusts must stay in force", got.PeerTrust)
	}
}

func TestAnotherDeviceCannotClearAKeysLatchByDeletingIt(t *testing.T) {
	_, a, b := pair(t)
	_ = a.vault.UpdateData(ctx, func(v *domain.VaultData) error {
		v.Identities["k1"] = domain.SSHIdentity{ID: "k1", KeyType: "ed25519", NonExportable: true}
		return nil
	})
	a.sync()
	b.sync()
	reports := syncRounds(t, a, b,
		func(v *domain.VaultData) { delete(v.Identities, "k1") },
		func(v *domain.VaultData) {
			v.Identities["k1"] = domain.SSHIdentity{ID: "k1", KeyType: "ed25519", AllowPlugins: true}
		})
	if reports[0].Held != 1 || reports[1].Quarantined != 1 {
		t.Fatalf("A's pulls = %+v; the deletion waits, and the version without the latch is refused", reports)
	}
	got, _ := a.vault.GetData()
	if k := got.Identities["k1"]; !k.NonExportable || k.AllowPlugins {
		t.Fatalf("k1 on A = %+v; sync never clears the latch or grants plugins a key", k)
	}
}
