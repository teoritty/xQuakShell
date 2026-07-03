package plugin_test

import (
	"testing"

	domainplugin "ssh-client/internal/domain/plugin"
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
