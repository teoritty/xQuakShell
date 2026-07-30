package discovery_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"xquakshell/internal/domain/discovery"
)

func baseNode(id string) discovery.Node {
	return discovery.Node{
		ID:    id,
		Kind:  discovery.KindInstance,
		Label: "container-1",
	}
}

func TestValidatePublishAcceptsValidSnapshot(t *testing.T) {
	children := []discovery.Node{
		baseNode("a"),
		{
			ID:    "b",
			Kind:  discovery.KindGroup,
			Label: "Volumes",
			Status: &discovery.Status{
				Tone:    discovery.ToneOK,
				Color:   "#1a2b3c",
				Tooltip: "healthy",
			},
			Actions: []discovery.Action{
				{ID: "start", Label: "Start"},
				{ID: "stop", Label: "Stop"},
			},
			DefaultActionID: "start",
		},
	}
	if err := discovery.ValidatePublish("", children); err != nil {
		t.Fatalf("expected valid snapshot, got %v", err)
	}
}

func TestValidatePublishRejectsEmptyID(t *testing.T) {
	n := baseNode("")
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for empty id, got %v", err)
	}
}

func TestValidatePublishRejectsTooLongID(t *testing.T) {
	n := baseNode(strings.Repeat("x", discovery.MaxIDLen+1))
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for too-long id, got %v", err)
	}
}

func TestValidatePublishAcceptsMaxLenID(t *testing.T) {
	n := baseNode(strings.Repeat("x", discovery.MaxIDLen))
	if err := discovery.ValidatePublish("", []discovery.Node{n}); err != nil {
		t.Fatalf("expected max-length id to be valid, got %v", err)
	}
}

func TestValidatePublishRejectsDuplicateID(t *testing.T) {
	children := []discovery.Node{baseNode("dup"), baseNode("dup")}
	err := discovery.ValidatePublish("", children)
	if !errors.Is(err, discovery.ErrInvalidNode) || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("expected duplicate id error, got %v", err)
	}
}

func TestValidatePublishRejectsInvalidKind(t *testing.T) {
	n := baseNode("a")
	n.Kind = "bogus"
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for invalid kind, got %v", err)
	}
}

func TestValidatePublishRejectsInvalidTone(t *testing.T) {
	n := baseNode("a")
	n.Status = &discovery.Status{Tone: "bogus"}
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for invalid tone, got %v", err)
	}
}

func TestValidatePublishRejectsInvalidColor(t *testing.T) {
	cases := []string{"123abc", "#abc", "#1a2b3c4d", "#gggggg"}
	for _, c := range cases {
		n := baseNode("a")
		n.Status = &discovery.Status{Tone: discovery.ToneOK, Color: c}
		if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
			t.Fatalf("color %q: expected ErrInvalidNode, got %v", c, err)
		}
	}
}

func TestValidatePublishAcceptsValidColor(t *testing.T) {
	n := baseNode("a")
	n.Status = &discovery.Status{Tone: discovery.ToneOK, Color: "#AaBbCc"}
	if err := discovery.ValidatePublish("", []discovery.Node{n}); err != nil {
		t.Fatalf("expected valid color to pass, got %v", err)
	}
}

func TestValidatePublishRejectsEmptyLabel(t *testing.T) {
	n := baseNode("a")
	n.Label = ""
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for empty label, got %v", err)
	}
}

func TestValidatePublishRejectsTooLongLabel(t *testing.T) {
	n := baseNode("a")
	n.Label = strings.Repeat("x", discovery.MaxLabelLen+1)
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for too-long label, got %v", err)
	}
}

func TestValidatePublishRejectsTooLongTooltip(t *testing.T) {
	n := baseNode("a")
	n.Status = &discovery.Status{Tone: discovery.ToneOK, Tooltip: strings.Repeat("x", discovery.MaxTooltipLen+1)}
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for too-long tooltip, got %v", err)
	}
}

func TestValidatePublishRejectsTooManyActions(t *testing.T) {
	n := baseNode("a")
	for i := 0; i < discovery.MaxActionsPerNode+1; i++ {
		n.Actions = append(n.Actions, discovery.Action{ID: fmt.Sprintf("action-%d", i)})
	}
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for too many actions, got %v", err)
	}
}

func TestValidatePublishRejectsDuplicateActionID(t *testing.T) {
	n := baseNode("a")
	n.Actions = []discovery.Action{{ID: "start"}, {ID: "start"}}
	err := discovery.ValidatePublish("", []discovery.Node{n})
	if !errors.Is(err, discovery.ErrInvalidNode) || !strings.Contains(err.Error(), "duplicate action") {
		t.Fatalf("expected duplicate action id error, got %v", err)
	}
}

func TestValidatePublishRejectsUnknownDefaultActionID(t *testing.T) {
	n := baseNode("a")
	n.Actions = []discovery.Action{{ID: "start"}}
	n.DefaultActionID = "stop"
	if err := discovery.ValidatePublish("", []discovery.Node{n}); !errors.Is(err, discovery.ErrInvalidNode) {
		t.Fatalf("expected ErrInvalidNode for unknown defaultActionId, got %v", err)
	}
}
