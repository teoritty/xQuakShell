package plugin

// NodeDetails is the property panel a discovery node shows (ADR-015 §3).
//
// It reuses the connection-field schema for the same reason a dialog does: the core already has a
// declarative form language with validation and a renderer, and a second one would be a second
// thing to keep correct.
type NodeDetails struct {
	Sections []FieldGroup      `json:"sections"`
	Values   map[string]string `json:"values"`
	// Editable false renders the panel read-only, which is the normal case for a node whose
	// properties are facts about a remote resource rather than local preferences.
	Editable bool `json:"editable"`
}

// Validate checks and prepares a details snapshot a plugin sent.
//
// The rules are the dialog's, and deliberately so: both are the same schema rendered in the same
// window, so a field that is refused in one and accepted in the other would be a difference with
// no meaning behind it. That is why both call one function rather than keeping a copy each.
func (d NodeDetails) Validate() error {
	return ValidateWireFields(d.Sections, "node details")
}

// FilterValues keeps only values whose field the snapshot declares.
//
// Used on the way back in, when the user saves: the plugin must never receive a value for a field
// it did not declare, and the frontend — which is not a trust boundary — sends whatever it
// rendered.
func (d NodeDetails) FilterValues(values map[string]string) map[string]string {
	declared := make(map[string]struct{})
	for _, group := range d.Sections {
		for _, field := range group.Fields {
			declared[field.ID] = struct{}{}
		}
	}
	out := make(map[string]string, len(values))
	for id, value := range values {
		if _, ok := declared[id]; ok {
			out[id] = value
		}
	}
	return out
}
