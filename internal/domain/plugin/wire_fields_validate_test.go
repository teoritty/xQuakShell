package plugin

import (
	"strings"
	"testing"
)

func wireSections(fields ...FieldDef) []FieldGroup {
	return []FieldGroup{{ID: "general", Label: "General", Fields: fields}}
}

// The reason this function exists: a schema off the wire has no compiled pattern, and without one
// every value of that field is refused by validateFieldValue with "field pattern not compiled".
func TestValidateWireFieldsCompilesPatterns(t *testing.T) {
	sections := wireSections(FieldDef{
		ID:         "name",
		Type:       FieldTypeText,
		Validation: &FieldValidation{Pattern: "^[a-z]+$"},
	})

	if err := ValidateWireFields(sections, "dialog"); err != nil {
		t.Fatalf("valid schema refused: %v", err)
	}

	re := sections[0].Fields[0].Validation.CompiledPattern()
	if re == nil {
		t.Fatal("pattern was not compiled into the caller's slice")
	}
	if !re.MatchString("abc") || re.MatchString("ABC") {
		t.Fatalf("compiled pattern does not behave like the declared one")
	}
}

func TestValidateWireFieldsRejectsUnsafePattern(t *testing.T) {
	cases := map[string]string{
		"nested quantifier": "((a+)+)+",
		"too long":          "^(" + strings.Repeat("a|", 600) + "b)$",
	}
	for name, pattern := range cases {
		t.Run(name, func(t *testing.T) {
			sections := wireSections(FieldDef{
				ID:         "f",
				Type:       FieldTypeText,
				Validation: &FieldValidation{Pattern: pattern},
			})
			if err := ValidateWireFields(sections, "dialog"); err == nil {
				t.Fatal("unsafe pattern accepted")
			}
		})
	}
}

func TestValidateWireFieldsRejectsInvalidRegexSyntax(t *testing.T) {
	sections := wireSections(FieldDef{
		ID:         "f",
		Type:       FieldTypeText,
		Validation: &FieldValidation{Pattern: "([unclosed"},
	})
	if err := ValidateWireFields(sections, "dialog"); err == nil {
		t.Fatal("uncompilable pattern accepted")
	}
}

func TestValidateWireFieldsRejectsDeclarationErrors(t *testing.T) {
	cases := map[string][]FieldGroup{
		"empty id":       wireSections(FieldDef{ID: "", Type: FieldTypeText}),
		"duplicate id":   wireSections(FieldDef{ID: "a", Type: FieldTypeText}, FieldDef{ID: "a", Type: FieldTypeText}),
		"secret":         wireSections(FieldDef{ID: "a", Type: FieldTypeText, Secret: true}),
		"password type":  wireSections(FieldDef{ID: "a", Type: FieldTypePassword}),
		"unknown type":   wireSections(FieldDef{ID: "a", Type: FieldType("colourpicker")}),
		"bad width":      wireSections(FieldDef{ID: "a", Type: FieldTypeText, Width: FieldWidth("quarter")}),
		"select no opts": wireSections(FieldDef{ID: "a", Type: FieldTypeSelect}),
		"unknown depends": wireSections(
			FieldDef{ID: "a", Type: FieldTypeText, DependsOn: "nowhere"},
		),
		"dependency cycle": wireSections(
			FieldDef{ID: "a", Type: FieldTypeText, DependsOn: "b"},
			FieldDef{ID: "b", Type: FieldTypeText, DependsOn: "a"},
		),
	}
	for name, sections := range cases {
		t.Run(name, func(t *testing.T) {
			if err := ValidateWireFields(sections, "dialog"); err == nil {
				t.Fatalf("%s accepted", name)
			}
		})
	}
}

// A dependsOn may point at a field declared later: the order of a panel is a layout decision, not
// a declaration order the plugin should have to reason about.
func TestValidateWireFieldsAllowsForwardDependency(t *testing.T) {
	sections := wireSections(
		FieldDef{ID: "path", Type: FieldTypeText, DependsOn: "mode"},
		FieldDef{ID: "mode", Type: FieldTypeText},
	)
	if err := ValidateWireFields(sections, "dialog"); err != nil {
		t.Fatalf("forward dependency refused: %v", err)
	}
}

func TestValidateWireFieldsEnforcesFieldLimit(t *testing.T) {
	fields := make([]FieldDef, 0, MaxDialogFields+1)
	for i := 0; i <= MaxDialogFields; i++ {
		fields = append(fields, FieldDef{ID: string(rune('a'+i%26)) + strings.Repeat("x", i), Type: FieldTypeText})
	}
	if err := ValidateWireFields(wireSections(fields...), "node details"); err == nil {
		t.Fatal("a schema past the field limit was accepted")
	}
}

// The two new ADR-015 types are part of the shared schema, so they must pass here as they do in a
// manifest.
func TestValidateWireFieldsAcceptsKeyValueAndCode(t *testing.T) {
	sections := wireSections(
		FieldDef{ID: "labels", Type: FieldTypeKeyValue},
		FieldDef{ID: "inspect", Type: FieldTypeCode},
	)
	if err := ValidateWireFields(sections, "node details"); err != nil {
		t.Fatalf("keyValue/code refused: %v", err)
	}
}
