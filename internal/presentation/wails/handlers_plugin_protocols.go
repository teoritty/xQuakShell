package wails

import domainplugin "ssh-client/internal/domain/plugin"

// FieldValidationDTO mirrors field validation rules for the UI.
type FieldValidationDTO struct {
	MinLength    int      `json:"minLength,omitempty"`
	MaxLength    int      `json:"maxLength,omitempty"`
	Min          *float64 `json:"min,omitempty"`
	Max          *float64 `json:"max,omitempty"`
	Pattern      string   `json:"pattern,omitempty"`
	MaxSizeBytes int      `json:"maxSizeBytes,omitempty"`
}

// FieldOptionDTO is a select option for the UI.
type FieldOptionDTO struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// FieldDefDTO declares a plugin connection field for the UI.
type FieldDefDTO struct {
	ID          string              `json:"id"`
	Label       string              `json:"label"`
	Type        string              `json:"type"`
	Required    bool                `json:"required"`
	Default     any                 `json:"default,omitempty"`
	Placeholder string              `json:"placeholder,omitempty"`
	Description string              `json:"description,omitempty"`
	Width       string              `json:"width,omitempty"`
	Order       int                 `json:"order"`
	Validation  *FieldValidationDTO `json:"validation,omitempty"`
	Options     []FieldOptionDTO    `json:"options,omitempty"`
	DependsOn   string              `json:"dependsOn,omitempty"`
	Secret      bool                `json:"secret"`
}

// FieldGroupDTO groups connection fields for the UI.
type FieldGroupDTO struct {
	ID     string        `json:"id"`
	Label  string        `json:"label"`
	Order  int           `json:"order"`
	Fields []FieldDefDTO `json:"fields"`
}

// ConnectionProtocolDTO is a plugin-contributed connection protocol.
type ConnectionProtocolDTO struct {
	ID          string          `json:"id"`
	Label       string          `json:"label"`
	DefaultPort int             `json:"defaultPort,omitempty"`
	Icon        string          `json:"icon,omitempty"`
	Surface     string          `json:"surface,omitempty"`
	RemoteFS    bool            `json:"remoteFs,omitempty"`
	Fields      []FieldGroupDTO `json:"fields,omitempty"`
}

// GetPluginConnectionProtocols returns merged protocol contributions for the UI.
func (a *AppAPI) GetPluginConnectionProtocols() []ConnectionProtocolDTO {
	if a.plugins == nil {
		return []ConnectionProtocolDTO{}
	}
	out := make([]ConnectionProtocolDTO, 0, 8)
	out = append(out, ConnectionProtocolDTO{
		ID:          "ssh",
		Label:       "SSH",
		DefaultPort: 22,
		Icon:        "terminal",
		RemoteFS:    true,
	})
	seen := map[string]struct{}{"ssh": {}}
	for _, p := range a.plugins.Registry().List() {
		surface := p.Manifest.SessionSurface()
		remoteFS := p.Manifest.Capabilities.Session != nil && p.Manifest.Capabilities.Session.RemoteFS
		for _, cp := range p.Manifest.Contributions.ConnectionProtocols {
			if _, ok := seen[cp.ID]; ok {
				continue
			}
			seen[cp.ID] = struct{}{}
			dto := mapConnectionProtocol(cp, surface)
			dto.RemoteFS = remoteFS
			out = append(out, dto)
		}
	}
	return out
}

func mapConnectionProtocol(p domainplugin.ConnectionProtocolContribution, surface string) ConnectionProtocolDTO {
	dto := ConnectionProtocolDTO{
		ID:          p.ID,
		Label:       p.Label,
		DefaultPort: p.DefaultPort,
		Icon:        p.Icon,
		Surface:     surface,
	}
	for _, group := range p.Fields {
		dto.Fields = append(dto.Fields, mapFieldGroup(group))
	}
	return dto
}

func mapFieldGroup(group domainplugin.FieldGroup) FieldGroupDTO {
	dto := FieldGroupDTO{
		ID:    group.ID,
		Label: group.Label,
		Order: group.Order,
	}
	for _, field := range group.Fields {
		dto.Fields = append(dto.Fields, mapFieldDef(field))
	}
	return dto
}

func mapFieldDef(field domainplugin.FieldDef) FieldDefDTO {
	dto := FieldDefDTO{
		ID:          field.ID,
		Label:       field.Label,
		Type:        string(field.Type),
		Required:    field.Required,
		Default:     field.Default,
		Placeholder: field.Placeholder,
		Description: field.Description,
		Width:       string(field.Width),
		Order:       field.Order,
		DependsOn:   field.DependsOn,
		Secret:      field.Secret,
	}
	if field.Validation != nil {
		dto.Validation = &FieldValidationDTO{
			MinLength:    field.Validation.MinLength,
			MaxLength:    field.Validation.MaxLength,
			Min:          field.Validation.Min,
			Max:          field.Validation.Max,
			Pattern:      field.Validation.Pattern,
			MaxSizeBytes: field.Validation.MaxSizeBytes,
		}
	}
	for _, opt := range field.Options {
		dto.Options = append(dto.Options, FieldOptionDTO{Value: opt.Value, Label: opt.Label})
	}
	return dto
}
