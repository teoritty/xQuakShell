package plugin_test

import (
	"testing"

	domainplugin "xquakshell/internal/domain/plugin"
)

func TestValidateManifestFieldsRejectsSecretDefault(t *testing.T) {
	m := &domainplugin.Manifest{
		Contributions: domainplugin.Contributions{
			ConnectionProtocols: []domainplugin.ConnectionProtocolContribution{
				{
					ID: "telnet",
					Fields: []domainplugin.FieldGroup{
						{
							ID:    "auth",
							Label: "Auth",
							Fields: []domainplugin.FieldDef{
								{
									ID:      "password",
									Label:   "Password",
									Type:    domainplugin.FieldTypePassword,
									Secret:  true,
									Default: "x",
								},
							},
						},
					},
				},
			},
		},
	}
	if err := domainplugin.ValidateManifestFields(m); err == nil {
		t.Fatal("expected secret default rejection")
	}
}

func TestValidateManifestFieldsRejectsCyclicDependsOn(t *testing.T) {
	m := &domainplugin.Manifest{
		Contributions: domainplugin.Contributions{
			ConnectionProtocols: []domainplugin.ConnectionProtocolContribution{
				{
					ID: "telnet",
					Fields: []domainplugin.FieldGroup{
						{
							ID:    "g",
							Label: "G",
							Fields: []domainplugin.FieldDef{
								{ID: "a", Label: "A", Type: domainplugin.FieldTypeText, Secret: false, DependsOn: "b"},
								{ID: "b", Label: "B", Type: domainplugin.FieldTypeText, Secret: false, DependsOn: "a"},
							},
						},
					},
				},
			},
		},
	}
	if err := domainplugin.ValidateManifestFields(m); err == nil {
		t.Fatal("expected cyclic dependsOn rejection")
	}
}

func TestIsFieldVisible(t *testing.T) {
	noDep := domainplugin.FieldDef{ID: "host", Type: domainplugin.FieldTypeText}
	withDep := domainplugin.FieldDef{ID: "password", Type: domainplugin.FieldTypePassword, DependsOn: "auth_type"}

	if !domainplugin.IsFieldVisible(noDep, map[string]string{}) {
		t.Fatal("field without dependsOn should be visible")
	}
	if domainplugin.IsFieldVisible(withDep, map[string]string{}) {
		t.Fatal("field with empty dependency should be hidden")
	}
	if domainplugin.IsFieldVisible(withDep, map[string]string{"auth_type": ""}) {
		t.Fatal("field with empty dependency value should be hidden")
	}
	if !domainplugin.IsFieldVisible(withDep, map[string]string{"auth_type": "password"}) {
		t.Fatal("field with truthy dependency should be visible")
	}
}

func TestValidateManifestFieldsCompilesRegexPattern(t *testing.T) {
	m := &domainplugin.Manifest{
		Contributions: domainplugin.Contributions{
			ConnectionProtocols: []domainplugin.ConnectionProtocolContribution{
				{
					ID: "telnet",
					Fields: []domainplugin.FieldGroup{
						{
							ID:    "g",
							Label: "G",
							Fields: []domainplugin.FieldDef{
								{
									ID:    "port",
									Label: "Port",
									Type:  domainplugin.FieldTypeText,
									Validation: &domainplugin.FieldValidation{
										Pattern: `^\d+$`,
									},
								},
								{
									ID:    "host",
									Label: "Host",
									Type:  domainplugin.FieldTypeText,
								},
							},
						},
					},
				},
			},
		},
	}
	if err := domainplugin.ValidateManifestFields(m); err != nil {
		t.Fatal(err)
	}
	withPattern := m.Contributions.ConnectionProtocols[0].Fields[0].Fields[0].Validation
	if withPattern.CompiledPattern() == nil {
		t.Fatal("expected compiled pattern for field with validation.pattern")
	}
	withoutPattern := m.Contributions.ConnectionProtocols[0].Fields[0].Fields[1].Validation
	if withoutPattern != nil && withoutPattern.CompiledPattern() != nil {
		t.Fatal("expected nil compiled pattern for field without validation")
	}
}

func TestValidateManifestFieldsRejectsDependsOnSecretField(t *testing.T) {
	m := &domainplugin.Manifest{
		Contributions: domainplugin.Contributions{
			ConnectionProtocols: []domainplugin.ConnectionProtocolContribution{
				{
					ID: "telnet",
					Fields: []domainplugin.FieldGroup{
						{
							ID:    "auth",
							Label: "Auth",
							Fields: []domainplugin.FieldDef{
								{ID: "password", Label: "Password", Type: domainplugin.FieldTypePassword, Secret: true},
								{ID: "confirm", Label: "Confirm", Type: domainplugin.FieldTypeText, DependsOn: "password"},
							},
						},
					},
				},
			},
		},
	}
	if err := domainplugin.ValidateManifestFields(m); err == nil {
		t.Fatal("expected dependsOn secret field rejection")
	}
}
