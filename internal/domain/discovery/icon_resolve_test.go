package discovery_test

import (
	"testing"

	"xquakshell/internal/domain/discovery"
)

func TestResolveIconOwnIconWins(t *testing.T) {
	node := discovery.Node{ID: "vol1", IconID: "volume-icon"}
	ancestors := []discovery.Node{{ID: "volumes", IconID: "volumes-icon"}}
	if got := discovery.ResolveIcon(node, ancestors); got != "volume-icon" {
		t.Fatalf("got %q, want own icon to win", got)
	}
}

func TestResolveIconSubgroupWithOwnIconDoesNotInheritParent(t *testing.T) {
	// The "volumes" subgroup declares its own icon, so it must not fall back to "docker"'s,
	// even though docker is a nearer-or-equal ancestor in spirit — this is the case ADR-014
	// calls out explicitly: inheritance is a fallback, not forced propagation.
	volumes := discovery.Node{ID: "volumes", IconID: "volumes-icon"}
	ancestors := []discovery.Node{{ID: "docker", IconID: "docker-icon"}}
	if got := discovery.ResolveIcon(volumes, ancestors); got != "volumes-icon" {
		t.Fatalf("got %q, want volumes' own icon", got)
	}
}

func TestResolveIconInheritsThroughTwoLevels(t *testing.T) {
	// instance -> volumes (no icon) -> docker (icon). Nearest ancestor without an icon is
	// skipped; the walk continues to the next one.
	instance := discovery.Node{ID: "vol1"}
	ancestors := []discovery.Node{
		{ID: "volumes"},                       // nearest, no icon
		{ID: "docker", IconID: "docker-icon"}, // root, has icon
	}
	if got := discovery.ResolveIcon(instance, ancestors); got != "docker-icon" {
		t.Fatalf("got %q, want inherited docker-icon", got)
	}
}

func TestResolveIconNoIconAnywhereReturnsEmpty(t *testing.T) {
	instance := discovery.Node{ID: "vol1"}
	ancestors := []discovery.Node{{ID: "volumes"}, {ID: "docker"}}
	if got := discovery.ResolveIcon(instance, ancestors); got != "" {
		t.Fatalf("got %q, want empty string", got)
	}
}

func TestResolveIconInheritsFromNearestAncestor(t *testing.T) {
	// Nearest ancestor has an icon, farther one has a different icon: nearest must win.
	instance := discovery.Node{ID: "vol1"}
	ancestors := []discovery.Node{
		{ID: "volumes", IconID: "volumes-icon"},
		{ID: "docker", IconID: "docker-icon"},
	}
	if got := discovery.ResolveIcon(instance, ancestors); got != "volumes-icon" {
		t.Fatalf("got %q, want nearest ancestor's icon", got)
	}
}
