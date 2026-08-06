package wails

import domainplugin "xquakshell/internal/domain/plugin"

// DialogFieldOptionDTO is one choice of a select field.
type DialogFieldOptionDTO struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// DialogFieldDTO is one field of a dialog or a node details panel.
//
// It is a projection of FieldDef rather than the type itself: `secret` is absent because a dialog
// field may not be one, and the compiled regex FieldValidation carries has no meaning in a
// browser. Sending the domain type directly would export both.
type DialogFieldDTO struct {
	ID          string                 `json:"id"`
	Label       string                 `json:"label"`
	Type        string                 `json:"type"`
	Required    bool                   `json:"required"`
	Placeholder string                 `json:"placeholder,omitempty"`
	Description string                 `json:"description,omitempty"`
	Width       string                 `json:"width,omitempty"`
	Order       int                    `json:"order"`
	DependsOn   string                 `json:"dependsOn,omitempty"`
	Options     []DialogFieldOptionDTO `json:"options,omitempty"`
	// Pattern, MinLength and the rest travel so the form can validate as the user types. The host
	// re-checks everything on submit regardless — the frontend is not a trust boundary.
	MinLength int      `json:"minLength,omitempty"`
	MaxLength int      `json:"maxLength,omitempty"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	Pattern   string   `json:"pattern,omitempty"`
}

// DialogSectionDTO groups fields for display.
type DialogSectionDTO struct {
	ID     string           `json:"id"`
	Label  string           `json:"label"`
	Order  int              `json:"order"`
	Fields []DialogFieldDTO `json:"fields"`
}

// DialogDTO is one open modal as the frontend sees it.
type DialogDTO struct {
	DialogID    string             `json:"dialogId"`
	PluginID    string             `json:"pluginId"`
	Kind        string             `json:"kind"`
	Title       string             `json:"title"`
	SubmitLabel string             `json:"submitLabel,omitempty"`
	Sections    []DialogSectionDTO `json:"sections"`
	Values      map[string]string  `json:"values"`
}

// DialogClosedPayload names the dialog that is gone.
type DialogClosedPayload struct {
	DialogID string `json:"dialogId"`
}

// DialogErrorPayload carries a plugin-reported failure on a dialog that stays open.
type DialogErrorPayload struct {
	DialogID    string            `json:"dialogId"`
	Message     string            `json:"message"`
	FieldErrors map[string]string `json:"fieldErrors"`
}

func dialogToDTO(d domainplugin.Dialog) DialogDTO {
	values := d.Values
	if values == nil {
		values = map[string]string{}
	}
	return DialogDTO{
		DialogID:    d.ID,
		PluginID:    d.PluginID,
		Kind:        string(d.Kind),
		Title:       d.Title,
		SubmitLabel: d.SubmitLabel,
		Sections:    fieldGroupsToDTO(d.Sections),
		Values:      values,
	}
}

func fieldGroupsToDTO(groups []domainplugin.FieldGroup) []DialogSectionDTO {
	out := make([]DialogSectionDTO, 0, len(groups))
	for _, group := range groups {
		fields := make([]DialogFieldDTO, 0, len(group.Fields))
		for _, field := range group.Fields {
			fields = append(fields, fieldToDTO(field))
		}
		out = append(out, DialogSectionDTO{
			ID:     group.ID,
			Label:  group.Label,
			Order:  group.Order,
			Fields: fields,
		})
	}
	return out
}

func fieldToDTO(field domainplugin.FieldDef) DialogFieldDTO {
	dto := DialogFieldDTO{
		ID:          field.ID,
		Label:       field.Label,
		Type:        string(field.Type),
		Required:    field.Required,
		Placeholder: field.Placeholder,
		Description: field.Description,
		Width:       string(field.Width),
		Order:       field.Order,
		DependsOn:   field.DependsOn,
	}
	for _, opt := range field.Options {
		dto.Options = append(dto.Options, DialogFieldOptionDTO{Value: opt.Value, Label: opt.Label})
	}
	if field.Validation != nil {
		dto.MinLength = field.Validation.MinLength
		dto.MaxLength = field.Validation.MaxLength
		dto.Min = field.Validation.Min
		dto.Max = field.Validation.Max
		dto.Pattern = field.Validation.Pattern
	}
	return dto
}
