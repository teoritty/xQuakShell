package plugin

import (
	"fmt"
	"regexp"
)

// ValidateManifestFields validates connection protocol field declarations.
func ValidateManifestFields(m *Manifest) error {
	for _, proto := range m.Contributions.ConnectionProtocols {
		if err := validateProtocolFields(proto); err != nil {
			return err
		}
	}
	return nil
}

func validateProtocolFields(proto ConnectionProtocolContribution) error {
	seenIDs := make(map[string]bool)
	for _, group := range proto.Fields {
		if group.ID == "" {
			return fmt.Errorf("%w: field group id is required in protocol %q", ErrInvalidManifest, proto.ID)
		}
		if group.Label == "" {
			return fmt.Errorf("%w: field group label is required in protocol %q", ErrInvalidManifest, proto.ID)
		}
		for _, field := range group.Fields {
			if field.ID == "" {
				return fmt.Errorf("%w: field id is required in protocol %q", ErrInvalidManifest, proto.ID)
			}
			if seenIDs[field.ID] {
				return fmt.Errorf("%w: duplicate field id %q in protocol %q", ErrInvalidManifest, field.ID, proto.ID)
			}
			seenIDs[field.ID] = true
		}
	}

	fieldByID := make(map[string]*FieldDef, len(seenIDs))
	for gi := range proto.Fields {
		for fi := range proto.Fields[gi].Fields {
			f := &proto.Fields[gi].Fields[fi]
			fieldByID[f.ID] = f
		}
	}

	fieldGraph := make(map[string][]string)

	for _, group := range proto.Fields {
		for i := range group.Fields {
			field := &group.Fields[i]

			if !isValidFieldType(field.Type) {
				return fmt.Errorf("%w: unknown field type %q in field %q", ErrInvalidManifest, field.Type, field.ID)
			}

			if field.Type == FieldTypeSelect && len(field.Options) == 0 {
				return fmt.Errorf("%w: select field %q must have options", ErrInvalidManifest, field.ID)
			}

			if field.Type == FieldTypePassword && !field.Secret {
				return fmt.Errorf("%w: password field %q must have secret=true", ErrInvalidManifest, field.ID)
			}

			// A secret's storage story is the vault, keyed by connection and field id. keyValue and
			// code have neither shape nor a place to be stored that way, so a "secret" one would be
			// a plaintext string wearing a lock icon (ADR-015 §2).
			if field.Secret && !FieldTypeAllowsSecret(field.Type) {
				return fmt.Errorf("%w: %s field %q may not be secret", ErrInvalidManifest, field.Type, field.ID)
			}

			if field.Width != "" && !isValidWidth(field.Width) {
				return fmt.Errorf("%w: invalid width %q for field %q", ErrInvalidManifest, field.Width, field.ID)
			}

			if field.Default != nil && !defaultMatchesType(field.Default, field.Type) {
				return fmt.Errorf("%w: default value type mismatch for field %q", ErrInvalidManifest, field.ID)
			}

			if field.Type == FieldTypeSelect && field.Default != nil {
				defaultStr := fmt.Sprintf("%v", field.Default)
				found := false
				for _, opt := range field.Options {
					if opt.Value == defaultStr {
						found = true
						break
					}
				}
				if !found {
					return fmt.Errorf("%w: default value %q not in options for field %q", ErrInvalidManifest, defaultStr, field.ID)
				}
			}

			if field.Secret && field.Default != nil {
				return fmt.Errorf("%w: secret field %q cannot have a default value", ErrInvalidManifest, field.ID)
			}

			if field.Validation != nil && field.Validation.Pattern != "" {
				compiled, err := validateRegexPatternSafe(field.Validation.Pattern)
				if err != nil {
					return fmt.Errorf("%w: invalid pattern in field %q: %v", ErrInvalidManifest, field.ID, err)
				}
				field.Validation.compiled = compiled
			}

			if field.Type == FieldTypeTextarea {
				if field.Validation == nil {
					field.Validation = &FieldValidation{}
				}
				if field.Validation.MaxSizeBytes == 0 {
					field.Validation.MaxSizeBytes = 1048576
				}
			}

			if field.DependsOn != "" {
				if !seenIDs[field.DependsOn] {
					return fmt.Errorf("%w: dependsOn %q references unknown field in field %q", ErrInvalidManifest, field.DependsOn, field.ID)
				}
				if depDef := fieldByID[field.DependsOn]; depDef != nil && depDef.Secret {
					return fmt.Errorf("%w: field %q cannot depend on secret field %q", ErrInvalidManifest, field.ID, field.DependsOn)
				}
				fieldGraph[field.ID] = append(fieldGraph[field.ID], field.DependsOn)
			}
		}
	}

	if err := checkDependencyCycles(fieldGraph); err != nil {
		return fmt.Errorf("%w: cyclic dependency detected in protocol %q: %v", ErrInvalidManifest, proto.ID, err)
	}

	return nil
}

func isValidFieldType(t FieldType) bool {
	switch t {
	case FieldTypeText, FieldTypePassword, FieldTypeNumber,
		FieldTypeSelect, FieldTypeCheckbox, FieldTypeTextarea,
		FieldTypeKeyValue, FieldTypeCode:
		return true
	}
	return false
}

func isValidWidth(w FieldWidth) bool {
	switch w {
	case WidthFull, WidthHalf, WidthThird:
		return true
	}
	return false
}

func defaultMatchesType(defaultVal any, fieldType FieldType) bool {
	switch fieldType {
	case FieldTypeText, FieldTypePassword, FieldTypeTextarea, FieldTypeSelect:
		_, ok := defaultVal.(string)
		return ok
	case FieldTypeNumber:
		switch defaultVal.(type) {
		case int, int32, int64, float32, float64:
			return true
		}
		return false
	case FieldTypeCheckbox:
		_, ok := defaultVal.(bool)
		return ok
	}
	return false
}

func validateRegexPatternSafe(pattern string) (*regexp.Regexp, error) {
	if len(pattern) > 1000 {
		return nil, fmt.Errorf("pattern too long (max 1000 chars)")
	}

	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("invalid regex syntax: %w", err)
	}

	if err := checkRegexSafety(re, pattern); err != nil {
		return nil, err
	}
	return re, nil
}

func checkRegexSafety(_ *regexp.Regexp, pattern string) error {
	depth := 0
	maxDepth := 0
	hasQuantifier := false

	for i, ch := range pattern {
		switch ch {
		case '(':
			depth++
			if depth > maxDepth {
				maxDepth = depth
			}
		case ')':
			depth--
			if i+1 < len(pattern) {
				nextCh := pattern[i+1]
				if nextCh == '+' || nextCh == '*' || nextCh == '?' {
					if hasQuantifier {
						return fmt.Errorf("nested quantifiers detected (ReDoS risk)")
					}
				}
			}
		case '+', '*':
			hasQuantifier = true
		case '{':
			if i+1 < len(pattern) && pattern[i+1] >= '0' && pattern[i+1] <= '9' {
				hasQuantifier = true
			}
		}
	}

	if maxDepth > 3 {
		return fmt.Errorf("regex nesting too deep (max 3 levels)")
	}

	return nil
}

func checkDependencyCycles(graph map[string][]string) error {
	visited := make(map[string]bool)
	recStack := make(map[string]bool)

	var dfs func(node string) error
	dfs = func(node string) error {
		visited[node] = true
		recStack[node] = true

		for _, dep := range graph[node] {
			if !visited[dep] {
				if err := dfs(dep); err != nil {
					return err
				}
			} else if recStack[dep] {
				return fmt.Errorf("cycle detected: %s -> %s", node, dep)
			}
		}

		recStack[node] = false
		return nil
	}

	for node := range graph {
		if !visited[node] {
			if err := dfs(node); err != nil {
				return err
			}
		}
	}

	return nil
}
