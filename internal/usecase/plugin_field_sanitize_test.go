package usecase

import (
	"strings"
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

// U+202E reverses everything after it in the rendered string without changing what is stored: the
// user reads one thing and confirms another. U+0007 is a C0 control. Both are what the discovery
// sanitizer exists to remove, and a field label sits in the same window as the tree row it was
// written for.
const bidiOverride = "‮"
const bellControl = ""

func sanitizeProbeGroups() []domainplugin.FieldGroup {
	return []domainplugin.FieldGroup{{
		ID:    "g",
		Label: "General" + bidiOverride,
		Fields: []domainplugin.FieldDef{{
			ID:          "mode",
			Label:       "Mode" + bidiOverride,
			Placeholder: "type here" + bellControl,
			Description: "explains" + bidiOverride,
			Type:        domainplugin.FieldTypeSelect,
			Options: []domainplugin.FieldOption{
				{Value: "rw" + bidiOverride, Label: "read-write" + bidiOverride},
			},
		}},
	}}
}

func containsUnsafe(s string) bool {
	return strings.Contains(s, bidiOverride) || strings.Contains(s, bellControl)
}

func TestSanitizeFieldGroupsCleansEveryDisplayedString(t *testing.T) {
	clean := sanitizeFieldGroups(sanitizeProbeGroups())

	group := clean[0]
	if containsUnsafe(group.Label) {
		t.Fatalf("section label was not cleaned: %q", group.Label)
	}
	field := group.Fields[0]
	for name, value := range map[string]string{
		"label":       field.Label,
		"placeholder": field.Placeholder,
		"description": field.Description,
		"option label": field.Options[0].Label,
	} {
		if containsUnsafe(value) {
			t.Fatalf("%s was not cleaned: %q", name, value)
		}
	}
}

// An option's value is data the plugin matches a submit against, not something drawn. Rewriting it
// would mean accepting an answer the plugin cannot recognise.
func TestSanitizeFieldGroupsLeavesOptionValuesAlone(t *testing.T) {
	clean := sanitizeFieldGroups(sanitizeProbeGroups())
	if clean[0].Fields[0].Options[0].Value != "rw"+bidiOverride {
		t.Fatalf("an option value must travel unchanged, got %q", clean[0].Fields[0].Options[0].Value)
	}
}

// The caller owns what it passed in. A validator that rewrote a plugin's declaration in place would
// make the same sections mean different things depending on how far they had travelled.
func TestSanitizeFieldGroupsDoesNotMutateItsInput(t *testing.T) {
	original := sanitizeProbeGroups()
	_ = sanitizeFieldGroups(original)
	if !containsUnsafe(original[0].Fields[0].Label) {
		t.Fatal("the input schema was modified in place")
	}
}

func TestSanitizeMessageCleansAndBounds(t *testing.T) {
	if got := sanitizeMessage("refused" + bidiOverride); containsUnsafe(got) {
		t.Fatalf("message was not cleaned: %q", got)
	}
	long := strings.Repeat("x", domainplugin.MaxPluginMessageLen+50)
	if got := sanitizeMessage(long); len([]rune(got)) != domainplugin.MaxPluginMessageLen {
		t.Fatalf("message length = %d, want %d", len([]rune(got)), domainplugin.MaxPluginMessageLen)
	}
	// A message explains a refusal, so it has more room than a title, which only labels something.
	if domainplugin.MaxPluginMessageLen <= domainplugin.MaxSurfaceTitleLen {
		t.Fatal("a message must not be bounded as tightly as a title")
	}
}

func TestSanitizeMessagesKeepsFieldIDs(t *testing.T) {
	clean := sanitizeMessages(map[string]string{"name": "bad" + bidiOverride})
	if _, ok := clean["name"]; !ok {
		t.Fatalf("a field id is matched against a declaration, not displayed: %v", clean)
	}
	if containsUnsafe(clean["name"]) {
		t.Fatalf("per-field message was not cleaned: %q", clean["name"])
	}
}

func TestSanitizeHandlesEmptyInput(t *testing.T) {
	if got := sanitizeFieldGroups(nil); got != nil {
		t.Fatalf("nil in, nil out: %v", got)
	}
	if got := sanitizeMessages(nil); got != nil {
		t.Fatalf("nil in, nil out: %v", got)
	}
	if got := sanitizeMessage(""); got != "" {
		t.Fatalf("empty in, empty out: %q", got)
	}
}
