package usecase

import (
	"strings"

	"xquakshell/internal/domain/discovery"
	domainplugin "xquakshell/internal/domain/plugin"
)

// Sanitizing plugin-authored display strings (ADR-015 §Security model).
//
// Titles were the only strings being cleaned, which left every other string a plugin draws — field
// labels, descriptions, placeholders, select option labels, error messages — going to the renderer
// as sent. There is no XSS in that: nothing a plugin sends is interpolated as HTML. What there is
// is a bidirectional override, which reorders what the user reads without changing what is stored,
// in a form they are about to press a button on. That is the attack ADR-014 introduced the
// sanitizer for, and it does not stop being one because the string is a label rather than a title.
//
// This lives in the use case layer, beside the title sanitizer, because domain/plugin deliberately
// does not import domain/discovery (see the note in field_code.go).

// sanitizeFieldGroups returns a cleaned copy of a field schema.
//
// A copy, not an in-place edit: the caller owns what it passed in, and a validator that quietly
// rewrote a plugin's declaration would make the same sections mean different things depending on
// how far through the pipeline they had travelled.
//
// One value is deliberately left alone: a `code` field's content. Control characters are the point
// of a monospace block — tabs and newlines are what makes it readable — and its own validator
// already refuses bidirectional overrides while allowing the rest (field_code.go).
func sanitizeFieldGroups(groups []domainplugin.FieldGroup) []domainplugin.FieldGroup {
	if len(groups) == 0 {
		return groups
	}
	out := make([]domainplugin.FieldGroup, len(groups))
	for i, group := range groups {
		out[i] = group
		out[i].Label = discovery.SanitizeText(group.Label)
		out[i].Fields = sanitizeFields(group.Fields)
	}
	return out
}

func sanitizeFields(fields []domainplugin.FieldDef) []domainplugin.FieldDef {
	if len(fields) == 0 {
		return fields
	}
	out := make([]domainplugin.FieldDef, len(fields))
	for i, field := range fields {
		out[i] = field
		out[i].Label = discovery.SanitizeText(field.Label)
		out[i].Placeholder = discovery.SanitizeText(field.Placeholder)
		out[i].Description = discovery.SanitizeText(field.Description)
		if len(field.Options) > 0 {
			options := make([]domainplugin.FieldOption, len(field.Options))
			for j, opt := range field.Options {
				// Only the label: the value is data the plugin matches on, and rewriting it would
				// mean accepting a submit the plugin then cannot recognise.
				options[j] = domainplugin.FieldOption{Value: opt.Value, Label: discovery.SanitizeText(opt.Label)}
			}
			out[i].Options = options
		}
	}
	return out
}

// sanitizeMessage cleans a plugin-authored message shown to the user and bounds its length.
//
// A separate bound from a title's: a title labels a tab and 128 characters is generous for one,
// while a message explains a refusal and cutting it at a title's length turns an explanation into
// a fragment.
func sanitizeMessage(message string) string {
	clean := discovery.SanitizeText(strings.TrimSpace(message))
	runes := []rune(clean)
	if len(runes) > domainplugin.MaxPluginMessageLen {
		runes = runes[:domainplugin.MaxPluginMessageLen]
	}
	return string(runes)
}

// sanitizeMessages cleans a map of per-field messages, keeping the keys as they are: a field id is
// matched against a declaration, not displayed.
func sanitizeMessages(messages map[string]string) map[string]string {
	if len(messages) == 0 {
		return messages
	}
	out := make(map[string]string, len(messages))
	for id, message := range messages {
		out[id] = sanitizeMessage(message)
	}
	return out
}
