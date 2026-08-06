package plugin

import (
	"fmt"
	"strings"
)

// DialogKind distinguishes a question from a presentation (ADR-015 §2).
type DialogKind string

const (
	// DialogKindForm has submit and cancel and returns values.
	DialogKindForm DialogKind = "form"
	// DialogKindDetail has only a close button and never sends dialog.submit. Submitting one would
	// hand a plugin an answer to a question it never asked.
	DialogKindDetail DialogKind = "detail"
)

// ParseDialogKind converts a wire value to a DialogKind.
func ParseDialogKind(s string) (DialogKind, error) {
	switch DialogKind(strings.TrimSpace(s)) {
	case DialogKindForm:
		return DialogKindForm, nil
	case DialogKindDetail:
		return DialogKindDetail, nil
	default:
		return "", fmt.Errorf("unknown dialog kind %q", s)
	}
}

// Dialog is one open modal owned by a plugin.
//
// It carries the declared sections, not merely their ids, because they are what a submitted value
// is validated against: the frontend renders what it was given, and the host must be able to
// answer "was this field declared, and is this value allowed for it" without asking the frontend.
type Dialog struct {
	ID          string
	PluginID    string
	Kind        DialogKind
	Title       string
	SubmitLabel string
	Sections    []FieldGroup
	Values      map[string]string
}

// FieldByID finds a declared field. Absent means the value must not be accepted for it.
func (d Dialog) FieldByID(id string) (FieldDef, bool) {
	for _, group := range d.Sections {
		for _, field := range group.Fields {
			if field.ID == id {
				return field, true
			}
		}
	}
	return FieldDef{}, false
}

// CountFields totals the declared fields across every section.
func (d Dialog) CountFields() int {
	n := 0
	for _, group := range d.Sections {
		n += len(group.Fields)
	}
	return n
}
