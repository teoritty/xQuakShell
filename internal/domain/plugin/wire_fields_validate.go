package plugin

import "fmt"

// ValidateWireFields checks and PREPARES a field schema that arrived over the wire — a dialog's
// sections or a discovery node's details panel (ADR-015 §2, §3).
//
// Preparing is the half that cannot be done anywhere else. FieldValidation.compiled is unexported
// and is filled at manifest load by validateProtocolFields; a schema unmarshalled from JSON has it
// nil, and validateFieldValue answers "field pattern not compiled" for every value of a field that
// declared a pattern. So a wire schema must go through the same compilation, and the only package
// that can write that field is this one.
//
// The rules are the manifest's, minus the ones that only make sense for a stored connection: no
// secret (a dialog has no vault), no password type, no default (the caller sends values instead).
// A plugin should not have to learn two dialects of one schema.
//
// subject names the caller ("dialog", "node details") so a refusal says which surface it came from.
func ValidateWireFields(sections []FieldGroup, subject string) error {
	declared, err := collectWireFieldIDs(sections, subject)
	if err != nil {
		return err
	}
	if len(declared) > MaxDialogFields {
		return fmt.Errorf("%s: %d fields exceeds the limit of %d", subject, len(declared), MaxDialogFields)
	}

	graph := make(map[string][]string)
	for gi := range sections {
		for fi := range sections[gi].Fields {
			// By index, because compiling a pattern writes back into the caller's slice: the
			// compiled expression is what validateFieldValue will use on submit.
			field := &sections[gi].Fields[fi]
			if err := validateWireField(field, subject); err != nil {
				return err
			}
			if field.DependsOn == "" {
				continue
			}
			if !declared[field.DependsOn] {
				return fmt.Errorf("%s: field %q depends on unknown field %q", subject, field.ID, field.DependsOn)
			}
			graph[field.ID] = append(graph[field.ID], field.DependsOn)
		}
	}

	if err := checkDependencyCycles(graph); err != nil {
		return fmt.Errorf("%s: cyclic field dependency: %v", subject, err)
	}
	return nil
}

// collectWireFieldIDs gathers every declared id, refusing empties and duplicates. It runs before
// anything else so a dependsOn can be resolved against the whole schema rather than only against
// the fields declared above it — a panel is not a program and its order is a layout decision.
func collectWireFieldIDs(sections []FieldGroup, subject string) (map[string]bool, error) {
	declared := make(map[string]bool)
	for _, group := range sections {
		for _, field := range group.Fields {
			if field.ID == "" {
				return nil, fmt.Errorf("%s: field id must not be empty", subject)
			}
			if declared[field.ID] {
				return nil, fmt.Errorf("%s: duplicate field id %q", subject, field.ID)
			}
			declared[field.ID] = true
		}
	}
	return declared, nil
}

// validateWireField checks one field and compiles its pattern.
func validateWireField(field *FieldDef, subject string) error {
	if !IsValidDialogFieldType(field.Type) {
		return fmt.Errorf("%s: field %q has an unsupported type %q", subject, field.ID, field.Type)
	}
	if field.Secret {
		// A secret's storage story is the vault, keyed by connection and field id. Neither a dialog
		// nor a node details panel has a connection or persistence, so a "secret" field here would
		// be a plaintext string wearing a lock icon. vault.getSecret remains the way, under its own
		// consent (ADR-015 §2).
		return fmt.Errorf("%s: field %q may not be secret", subject, field.ID)
	}
	if field.Width != "" && !isValidWidth(field.Width) {
		return fmt.Errorf("%s: field %q has an invalid width %q", subject, field.ID, field.Width)
	}
	if field.Type == FieldTypeSelect && len(field.Options) == 0 {
		return fmt.Errorf("%s: select field %q must have options", subject, field.ID)
	}
	if field.Validation == nil || field.Validation.Pattern == "" {
		return nil
	}
	// The same safety check a manifest pattern gets: length, syntax, and the nesting/quantifier
	// screen. A pattern from a running plugin is no more trusted than one from its manifest.
	compiled, err := validateRegexPatternSafe(field.Validation.Pattern)
	if err != nil {
		return fmt.Errorf("%s: field %q has an invalid pattern: %v", subject, field.ID, err)
	}
	field.Validation.compiled = compiled
	return nil
}
