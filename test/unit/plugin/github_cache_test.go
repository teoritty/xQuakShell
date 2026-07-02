package plugin_test

import (
	"context"
	"testing"
	"time"

	infracache "ssh-client/internal/infra/cache"
)

func TestMemoryCache_SetGetTTL(t *testing.T) {
	cache := infracache.NewMemoryCache(time.Hour)
	ctx := context.Background()

	if err := cache.Set(ctx, "key", "value"); err != nil {
		t.Fatal(err)
	}
	got, ok, err := cache.Get(ctx, "key")
	if err != nil || !ok || got.(string) != "value" {
		t.Fatalf("unexpected cache result: %v %v %v", got, ok, err)
	}
}

func TestMemoryCache_Expires(t *testing.T) {
	cache := infracache.NewMemoryCache(time.Hour)
	ctx := context.Background()
	if err := cache.SetWithTTL(ctx, "key", "value", 10*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	_, ok, err := cache.Get(ctx, "key")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected expired entry")
	}
}
