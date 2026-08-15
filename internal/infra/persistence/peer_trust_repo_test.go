package persistence

import (
	"bytes"
	"context"
	"testing"

	"xquakshell/internal/domain"
)

// memVault is declared in connection_repo_test.go - same package, same fake. A second one would
// not only fail to compile, it would mean two different ideas of one storage.
func newTrustRepo() (*PeerTrustRepo, *memVault) {
	v := &memVault{data: domain.NewVaultData()}
	return NewPeerTrustRepo(v), v
}

func TestPeerTrustPutThenFind(t *testing.T) {
	repo, _ := newTrustRepo()
	entry := domain.PeerTrustEntry{Scope: "com.example.rdp", Subject: "h:3389", Material: []byte{1, 2, 3}}
	if err := repo.Put(context.Background(), entry); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := repo.Find("com.example.rdp", "h:3389")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got == nil {
		t.Fatal("the entry was not found")
	}
	if !bytes.Equal(got.Material, entry.Material) {
		t.Fatalf("material = % x", got.Material)
	}
	if got.AddedAt.IsZero() {
		t.Fatal("no timestamp was stamped; the trust list would have nothing to show")
	}
}

func TestPeerTrustFindMissingReturnsNil(t *testing.T) {
	repo, _ := newTrustRepo()
	got, err := repo.Find("s", "x")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got != nil {
		t.Fatalf("an unknown subject returned an entry: %+v", got)
	}
}

// The scope isolates entries: a plugin must not see another's trust even for the same subject.
func TestPeerTrustScopesAreIsolated(t *testing.T) {
	repo, _ := newTrustRepo()
	ctx := context.Background()
	if err := repo.Put(ctx, domain.PeerTrustEntry{Scope: "a", Subject: "h:1", Material: []byte{1}}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, err := repo.Find("b", "h:1")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got != nil {
		t.Fatal("an entry from one scope is visible from another")
	}
}

// A repeated Put replaces the entry rather than adding a second: otherwise both materials would
// count as trusted after a change, and a key rotation would stop meaning anything.
func TestPeerTrustPutReplaces(t *testing.T) {
	repo, _ := newTrustRepo()
	ctx := context.Background()
	base := domain.PeerTrustEntry{Scope: "a", Subject: "h:1", Material: []byte{1}}
	if err := repo.Put(ctx, base); err != nil {
		t.Fatalf("Put: %v", err)
	}
	base.Material = []byte{2}
	if err := repo.Put(ctx, base); err != nil {
		t.Fatalf("Put: %v", err)
	}
	list, err := repo.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("entries = %d, want 1", len(list))
	}
	if !bytes.Equal(list[0].Material, []byte{2}) {
		t.Fatalf("the material was not replaced: % x", list[0].Material)
	}
}

// A replacement must not disturb other subjects in the same scope.
func TestPeerTrustPutKeepsOtherSubjects(t *testing.T) {
	repo, _ := newTrustRepo()
	ctx := context.Background()
	for _, subject := range []string{"h:1", "h:2"} {
		if err := repo.Put(ctx, domain.PeerTrustEntry{Scope: "a", Subject: subject, Material: []byte{1}}); err != nil {
			t.Fatalf("Put %s: %v", subject, err)
		}
	}
	if err := repo.Put(ctx, domain.PeerTrustEntry{Scope: "a", Subject: "h:1", Material: []byte{9}}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	other, err := repo.Find("a", "h:2")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if other == nil {
		t.Fatal("replacing one subject deleted another")
	}
}

func TestPeerTrustRejectsOversizedAndEmptyMaterial(t *testing.T) {
	repo, _ := newTrustRepo()
	ctx := context.Background()
	big := domain.PeerTrustEntry{Scope: "a", Subject: "h", Material: make([]byte, domain.MaxPeerTrustMaterial+1)}
	if err := repo.Put(ctx, big); err == nil {
		t.Fatal("material over the ceiling was accepted")
	}
	empty := domain.PeerTrustEntry{Scope: "a", Subject: "h"}
	if err := repo.Put(ctx, empty); err == nil {
		t.Fatal("empty material was accepted")
	}
}

func TestPeerTrustRemoveIsIdempotent(t *testing.T) {
	repo, _ := newTrustRepo()
	ctx := context.Background()
	if err := repo.Put(ctx, domain.PeerTrustEntry{Scope: "a", Subject: "h", Material: []byte{1}}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := repo.Remove(ctx, "a", "h"); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	got, err := repo.Find("a", "h")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got != nil {
		t.Fatal("the entry survived its removal")
	}
	if err := repo.Remove(ctx, "a", "h"); err != nil {
		t.Fatalf("a repeated Remove: %v", err)
	}
}

// An older vault has no such field at all. That is normal, not a reason to fail.
func TestPeerTrustHandlesVaultWithoutField(t *testing.T) {
	v := &memVault{data: domain.NewVaultData()}
	v.data.PeerTrust = nil
	repo := NewPeerTrustRepo(v)

	got, err := repo.Find("a", "h")
	if err != nil {
		t.Fatalf("Find: %v", err)
	}
	if got != nil {
		t.Fatal("something was found in empty storage")
	}
	if err := repo.Put(context.Background(), domain.PeerTrustEntry{Scope: "a", Subject: "h", Material: []byte{1}}); err != nil {
		t.Fatalf("Put into a vault without the field: %v", err)
	}
}

// The repository must satisfy the domain port: without this it cannot be handed to the service,
// and the compiler stays quiet until the wiring itself.
func TestPeerTrustRepoSatisfiesPort(t *testing.T) {
	var _ domain.PeerTrustRepository = NewPeerTrustRepo(&memVault{data: domain.NewVaultData()})
}
