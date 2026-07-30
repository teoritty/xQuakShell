package discovery_test

import (
	"testing"

	"xquakshell/internal/domain/discovery"
)

func TestPluginMayPublishStateAllowsLoadingReadyError(t *testing.T) {
	for _, s := range []discovery.BranchState{discovery.BranchLoading, discovery.BranchReady, discovery.BranchError} {
		if !discovery.PluginMayPublishState(s) {
			t.Fatalf("expected %q to be publishable by a plugin", s)
		}
	}
}

func TestPluginMayPublishStateForbidsStale(t *testing.T) {
	if discovery.PluginMayPublishState(discovery.BranchStale) {
		t.Fatal("expected stale to be rejected: it is host-only state")
	}
}

func TestPluginMayPublishStateForbidsUnknownValue(t *testing.T) {
	if discovery.PluginMayPublishState(discovery.BranchState("bogus")) {
		t.Fatal("expected unknown branch state to be rejected")
	}
}
