package usecase

import (
	"errors"
	"testing"
)

// The bug these tests exist for: nothing in the app ever bound a discovery plugin to a session.
//
// The session bridge binds a plugin to the session it OWNS — the plugin that provides the protocol
// — and a discovery plugin owns nothing: it draws under a core SSH connection somebody else
// created. So its process was never started and its session never bound, and every discovery.publish
// it sent was refused by the IDOR check in PluginSessionRPCHandler. Every layer was correct on its
// own and the feature could not work at all.
//
// These assert on the grants themselves rather than on a rendered tree, because the grant is what
// the IDOR check reads.

func bindingsOf(pairs ...[2]string) []discoveryBinding {
	out := make([]discoveryBinding, 0, len(pairs))
	for _, pair := range pairs {
		out = append(out, discoveryBinding{pluginID: pair[0], sessionID: pair[1]})
	}
	return out
}

func sameBindings(got, want []discoveryBinding) bool {
	if len(got) != len(want) {
		return false
	}
	for _, w := range want {
		if !containsBinding(got, w) {
			return false
		}
	}
	return true
}

// TestLeadingSessionStartsAndAuthorizesEveryAddressablePlugin is the fix itself: a connection that
// reaches ready must leave every discovery plugin declaring its protocol both running and
// authorized for that session.
//
// The protocol filter is asserted in the same test rather than a separate one, because "bind
// everybody" would pass a test that only checked the ssh plugin — and binding a plugin to a
// connection it never claimed to understand hands out an authorization nothing asked for.
func TestLeadingSessionStartsAndAuthorizesEveryAddressablePlugin(t *testing.T) {
	h := newDiscoveryHarness(t,
		DiscoveryPluginTarget{PluginID: "ssh-one", ParentProtocols: []string{"ssh"}},
		DiscoveryPluginTarget{PluginID: "ssh-two", ParentProtocols: []string{"ssh", "telnet"}},
		DiscoveryPluginTarget{PluginID: "other", ParentProtocols: []string{"serial"}},
	)
	h.sessionReady("s1", "c1")

	want := bindingsOf([2]string{"ssh-one", "s1"}, [2]string{"ssh-two", "s1"})
	h.eventually(t, "both ssh plugins are authorized for the leading session", func() bool {
		return sameBindings(h.runtime.live(), want)
	})
	for _, pluginID := range []string{"ssh-one", "ssh-two"} {
		if !containsString(h.runtime.startedPlugins(), pluginID) {
			t.Fatalf("%s was authorized but never started; a bound plugin that is not running publishes nothing", pluginID)
		}
	}
	if containsString(h.runtime.startedPlugins(), "other") {
		t.Fatalf("a plugin that did not declare this connection's protocol must not be started, got %v", h.runtime.startedPlugins())
	}
}

// TestHandoverMovesTheAuthorizationToTheNewLeader: the old session's grant must not survive it.
// Leaving it behind is not merely untidy — it is a live authorization to publish into a session
// that no longer leads, which is exactly the IDOR the gate exists to refuse.
func TestHandoverMovesTheAuthorizationToTheNewLeader(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.sessionReady("s2", "c1")
	h.eventually(t, "the first leader is authorized", func() bool {
		return sameBindings(h.runtime.live(), bindingsOf([2]string{"p1", "s1"}))
	})

	h.leader.SessionClosed("s1", "c1")

	h.eventually(t, "the authorization moved to the new leader", func() bool {
		return sameBindings(h.runtime.live(), bindingsOf([2]string{"p1", "s2"}))
	})
	if !h.runtime.wasUnbound(discoveryBinding{pluginID: "p1", sessionID: "s1"}) {
		t.Fatal("the former leader's authorization was never released")
	}
}

// TestLastReadySessionLeavingReleasesEveryAuthorization: the tree is deleted when no ready session
// remains, and the grants that fed it must go with it.
func TestLastReadySessionLeavingReleasesEveryAuthorization(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.eventually(t, "the plugin is authorized", func() bool {
		return len(h.runtime.live()) == 1
	})

	h.leader.SessionClosed("s1", "c1")

	h.eventually(t, "every authorization is released", func() bool {
		return len(h.runtime.live()) == 0
	})
}

// TestANonLeadingSessionIsNeverAuthorized pins the leading-session rule at the level that matters.
// A plugin bound to two sessions of one connection could enumerate through either, which is the
// duplicate traffic the leading session exists to prevent — and one of the two is unobservable from
// the tree, since only the leader's publishes are accepted.
func TestANonLeadingSessionIsNeverAuthorized(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.sessionReady("s2", "c1")

	h.eventually(t, "the leader is authorized", func() bool {
		return sameBindings(h.runtime.live(), bindingsOf([2]string{"p1", "s1"}))
	})
	// Give the second SessionReady's reconciliation, if any, room to be wrong.
	h.leader.syncBindings("c1")
	if got := h.runtime.live(); !sameBindings(got, bindingsOf([2]string{"p1", "s1"})) {
		t.Fatalf("only the leading session may be authorized, got %+v", got)
	}
}

// TestAPluginThatWillNotStartIsNotAuthorized: binding a process that failed to start would grant a
// standing authorization to something that may come up much later, for a session that may be long
// gone by then. It is retried on the next reconciliation instead.
func TestAPluginThatWillNotStartIsNotAuthorized(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.runtime.startErr = errors.New("binary is missing")
	h.sessionReady("s1", "c1")

	h.eventually(t, "the start was attempted", func() bool {
		return len(h.runtime.startedPlugins()) > 0
	})
	if got := h.runtime.live(); len(got) != 0 {
		t.Fatalf("a plugin that would not start must not be authorized, got %+v", got)
	}
}

// TestHoldsBindingsFollowsTheGrants is what the host asks to decide whether a plugin may be
// suspended or abandoned. It must answer for the plugin, not for the connection: the same plugin
// may be drawing under several connections, and losing one of them does not make it idle.
func TestHoldsBindingsFollowsTheGrants(t *testing.T) {
	h := newDiscoveryHarness(t)
	if h.leader.HoldsBindings("p1") {
		t.Fatal("nothing is bound yet")
	}

	h.sessionReady("s1", "c1")
	h.sessionReady("s2", "c2")
	h.eventually(t, "both connections are authorized", func() bool {
		return len(h.runtime.live()) == 2
	})
	if !h.leader.HoldsBindings("p1") {
		t.Fatal("a plugin drawing under two connections must count as held")
	}
	if h.leader.HoldsBindings("someone-else") {
		t.Fatal("a plugin that holds nothing must not be reported as held")
	}

	h.leader.SessionClosed("s1", "c1")
	h.leader.awaitReconcile()
	if !h.leader.HoldsBindings("p1") {
		t.Fatal("losing one connection must not make a plugin idle while another still holds it")
	}

	h.leader.SessionClosed("s2", "c2")
	h.leader.awaitReconcile()
	if h.leader.HoldsBindings("p1") {
		t.Fatal("with every connection gone the plugin holds nothing and may be reclaimed")
	}
}

// TestReconcilingTwiceDoesNotRebindWhatIsAlreadyBound. The reconciliation is level-triggered and
// runs on every lifecycle event, so it runs often; each run must be a no-op when nothing changed.
// Re-binding would be harmless today and is still worth pinning: it is the difference between a
// reconciliation and a stream of edges, and the audit log records every bind.
func TestReconcilingTwiceDoesNotRebindWhatIsAlreadyBound(t *testing.T) {
	h := newDiscoveryHarness(t)
	h.sessionReady("s1", "c1")
	h.eventually(t, "the plugin is authorized", func() bool {
		return len(h.runtime.live()) == 1
	})

	before := len(h.runtime.startedPlugins())
	h.leader.syncBindings("c1")

	if got := len(h.runtime.startedPlugins()); got != before {
		t.Fatalf("an unchanged connection must not restart its plugins: %d -> %d", before, got)
	}
	if h.runtime.wasUnbound(discoveryBinding{pluginID: "p1", sessionID: "s1"}) {
		t.Fatal("an unchanged connection must not release and re-grant its authorization")
	}
}
