package plugin

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
	Aliases     []string         `json:"aliases,omitempty"`
}

// FieldValidation holds value constraints enforced by host on save.
type FieldValidation struct {
	MinLength    int      `json:"minLength,omitempty"`
	MaxLength    int      `json:"maxLength,omitempty"`
	Min          *float64 `json:"min,omitempty"`
	Max          *float64 `json:"max,omitempty"`
	Pattern      string   `json:"pattern,omitempty"`
	MaxSizeBytes int      `json:"maxSizeBytes,omitempty"`
}

// FieldOption is a select field choice.
type FieldOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}
