package plugin_test

import (
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
)

func TestMayPublishToEventChannel(t *testing.T) {
	const id = "com.example.plugin"
	cases := []struct {
		channel string
		want    bool
	}{
		{"core.session.opened", false},
		{"core.evil", false},
		{"plugin." + id, true},
		{"plugin." + id + ".events", true},
		{"plugin." + id + ".events.foo", true},
		{"plugin.com.other.events", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := domainplugin.MayPublishToEventChannel(id, tc.channel); got != tc.want {
			t.Fatalf("MayPublishToEventChannel(%q, %q) = %v, want %v", id, tc.channel, got, tc.want)
		}
	}
}

func TestOwnsPluginEventChannelSubscribe(t *testing.T) {
	const id = "com.example.plugin"
	cases := []struct {
		channel string
		want    bool
	}{
		{"core.session.opened", true},
		{"plugin." + id, true},
		{"plugin." + id + ".events", true},
		{"plugin.com.other.events", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := domainplugin.OwnsPluginEventChannel(id, tc.channel); got != tc.want {
			t.Fatalf("OwnsPluginEventChannel(%q, %q) = %v, want %v", id, tc.channel, got, tc.want)
		}
	}
}

func TestValidatePluginEventPatternRejectsCrossPlugin(t *testing.T) {
	const id = "com.example.plugin"
	if err := domainplugin.ValidatePluginEventPattern(id, "plugin.com.other.*"); err == nil {
		t.Fatal("expected cross-plugin pattern rejection")
	}
	if err := domainplugin.ValidatePluginEventPattern(id, "plugin."+id+".*"); err != nil {
		t.Fatalf("expected own namespace pattern ok: %v", err)
	}
	if err := domainplugin.ValidatePluginEventPattern(id, "core.session.*"); err != nil {
		t.Fatalf("expected core subscribe pattern ok: %v", err)
	}
}
