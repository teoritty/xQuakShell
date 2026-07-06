package domain

import "testing"

func TestIsLoopbackBind(t *testing.T) {
	cases := map[string]bool{
		"":            true,
		"127.0.0.1":   true,
		"localhost":   true,
		"::1":         true,
		"0.0.0.0":     false,
		"192.168.1.1": false,
	}
	for addr, want := range cases {
		if got := IsLoopbackBind(addr); got != want {
			t.Fatalf("IsLoopbackBind(%q) = %v, want %v", addr, got, want)
		}
	}
}

func TestEffectiveBindAddress(t *testing.T) {
	if got := EffectiveBindAddress(""); got != "127.0.0.1" {
		t.Fatalf("EffectiveBindAddress empty = %q", got)
	}
	if got := EffectiveBindAddress("0.0.0.0"); got != "0.0.0.0" {
		t.Fatalf("EffectiveBindAddress explicit = %q", got)
	}
}
