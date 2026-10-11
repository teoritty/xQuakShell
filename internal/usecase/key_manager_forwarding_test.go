package usecase

import (
	"context"
	"testing"
)

func TestKeyForwardingConsentIsOffByDefaultAndFollowsThePolicy(t *testing.T) {
	ctx := context.Background()
	svc, repo, _ := newKeyManager(t, nil)
	plain, err := svc.Generate(ctx, ed25519Spec("plain"), "", KeyOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if plain.AllowForwarding {
		t.Fatal("a key must not be forwardable unless the user said so")
	}
	lent, err := svc.Generate(ctx, ed25519Spec("lent"), "", KeyOptions{AllowForwarding: true})
	if err != nil || !lent.AllowForwarding {
		t.Fatalf("a key created forwardable must say so: %+v, %v", lent, err)
	}
	if err := svc.SetPolicy(ctx, lent.ID, KeyOptions{AllowForwarding: false}); err != nil {
		t.Fatal(err)
	}
	stored, err := repo.Get(ctx, lent.ID)
	if err != nil || stored.AllowForwarding {
		t.Fatalf("withdrawing consent must persist: %+v, %v", stored, err)
	}
}
