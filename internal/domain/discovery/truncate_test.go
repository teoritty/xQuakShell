package discovery_test

import (
	"fmt"
	"testing"

	"xquakshell/internal/domain/discovery"
)

func makeNodes(n int) []discovery.Node {
	out := make([]discovery.Node, n)
	for i := range out {
		out[i] = baseNode(fmt.Sprintf("node-%d", i))
	}
	return out
}

func TestTruncateChildrenBelowLimit(t *testing.T) {
	children := makeNodes(discovery.MaxChildrenPerPublish - 1)
	got, truncated := discovery.TruncateChildren(children)
	if truncated != nil {
		t.Fatalf("expected no truncation below the limit, got %+v", truncated)
	}
	if len(got) != len(children) {
		t.Fatalf("expected all %d children returned, got %d", len(children), len(got))
	}
}

func TestTruncateChildrenAtExactLimit(t *testing.T) {
	children := makeNodes(discovery.MaxChildrenPerPublish)
	got, truncated := discovery.TruncateChildren(children)
	if truncated != nil {
		t.Fatalf("expected no truncation exactly at the limit, got %+v", truncated)
	}
	if len(got) != discovery.MaxChildrenPerPublish {
		t.Fatalf("expected %d children, got %d", discovery.MaxChildrenPerPublish, len(got))
	}
}

func TestTruncateChildrenOverLimit(t *testing.T) {
	children := makeNodes(discovery.MaxChildrenPerPublish + 1)
	got, truncated := discovery.TruncateChildren(children)
	if truncated == nil {
		t.Fatal("expected truncation one over the limit")
	}
	if truncated.Shown != discovery.MaxChildrenPerPublish || truncated.Total != discovery.MaxChildrenPerPublish+1 {
		t.Fatalf("unexpected Truncated: %+v", truncated)
	}
	if len(got) != discovery.MaxChildrenPerPublish {
		t.Fatalf("expected %d children returned, got %d", discovery.MaxChildrenPerPublish, len(got))
	}
}
