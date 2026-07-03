package plugin

import "regexp"

// FieldType identifies a declarative connection field widget type.
type FieldType string

const (
	FieldTypeText     FieldType = "text"
	FieldTypePassword FieldType = "password"
	FieldTypeNumber   FieldType = "number"
	FieldTypeSelect   FieldType = "select"
	FieldTypeCheckbox FieldType = "checkbox"
	FieldTypeTextarea FieldType = "textarea"
)

// FieldWidth controls layout width in the connection details form.
type FieldWidth string

const (
	WidthFull  FieldWidth = "full"
	WidthHalf  FieldWidth = "half"
	WidthThird FieldWidth = "third"
)

// FieldGroup groups related connection fields in the UI.
type FieldGroup struct {
	ID     string     `json:"id"`
	Label  string     `json:"label"`
	Order  int        `json:"order"`
	Fields []FieldDef `json:"fields"`
}

// FieldDef declares a single plugin connection field.
type FieldDef struct {
	ID          string           `json:"id"`
	Label       string           `json:"label"`
	Type        FieldType        `json:"type"`
	Required    bool             `json:"required"`
	Default     any              `json:"default,omitempty"`
	Placeholder string           `json:"placeholder,omitempty"`
	Description string           `json:"description,omitempty"`
	Width       FieldWidth       `json:"width,omitempty"`
	Order       int              `json:"order"`
	Validation  *FieldValidation `json:"validation,omitempty"`
	Options     []FieldOption    `json:"options,omitempty"`
	DependsOn   string           `json:"dependsOn,omitempty"`
	Secret      bool             `json:"secret"`
}

// FieldValidation holds value constraints enforced by host on save.
type FieldValidation struct {
	MinLength    int      `json:"minLength,omitempty"`
	MaxLength    int      `json:"maxLength,omitempty"`
	Min          *float64 `json:"min,omitempty"`
	Max          *float64 `json:"max,omitempty"`
	Pattern      string   `json:"pattern,omitempty"`
	MaxSizeBytes int      `json:"maxSizeBytes,omitempty"`

	// compiled is populated once by validateRegexPatternSafe at manifest
	// load time. Never set this manually; nil means "no pattern" or
	// "not yet validated" — callers must not assume it's non-nil.
	compiled *regexp.Regexp
}

// CompiledPattern returns the pattern compiled at manifest-load time, or nil.
func (v *FieldValidation) CompiledPattern() *regexp.Regexp {
	if v == nil {
		return nil
	}
	return v.compiled
}

// IsFieldVisible reports whether a field is currently visible given a flat
// snapshot of field values (id -> raw stored/incoming value; secret fields
// may be represented by their non-empty secret ref — only truthiness matters
// here, never the plaintext).
//
// This is the single source of truth for dependsOn evaluation on the host
// side. It intentionally mirrors, but does not share code with, the 2-line
// truthy check in frontend/src/lib/connectionDetails/PluginConnectionFields.svelte
// (isVisible). Keep both in sync manually if dependsOn semantics ever change —
// duplicating a 3-line predicate across Go/TS does not justify a codegen
// pipeline.
func IsFieldVisible(field FieldDef, values map[string]string) bool {
	if field.DependsOn == "" {
		return true
	}
	return values[field.DependsOn] != ""
}

// FieldOption is a select field choice.
type FieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
