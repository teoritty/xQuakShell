package unit

import (
	"context"
	"reflect"
	"testing"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/presentation/wails"
	"xquakshell/internal/usecase"
)

// Saving settings assigns the whole AppSettings struct, but AppSettingsDTO does not carry every
// field: plugin trust and the capability grants are established through install-time consent, not
// through the settings dialog. Anything the DTO drops therefore has to be carried across the save
// explicitly, or changing the theme wipes it.
//
// serverOwnedSettings is that list. A field belongs here when the settings dialog must not be able
// to set it; anything else must survive the DTO round-trip.
var serverOwnedSettings = map[string]bool{
	"Plugins": true,
	"Embed":   true,
}

// fillNonZero gives every field in v a value distinguishable from its zero value, so "came back
// empty" is unambiguous evidence that a field was dropped rather than a field that was never set.
func fillNonZero(v reflect.Value) {
	switch v.Kind() {
	case reflect.Bool:
		v.SetBool(true)
	case reflect.String:
		v.SetString("x")
	case reflect.Int, reflect.Int64:
		if v.Type() == reflect.TypeOf(time.Duration(0)) {
			v.SetInt(int64(time.Minute))
			return
		}
		v.SetInt(7)
	case reflect.Slice:
		elem := reflect.New(v.Type().Elem()).Elem()
		fillNonZero(elem)
		v.Set(reflect.Append(v, elem))
	case reflect.Map:
		val := reflect.New(v.Type().Elem()).Elem()
		fillNonZero(val)
		key := reflect.New(v.Type().Key()).Elem()
		fillNonZero(key)
		v.Set(reflect.MakeMap(v.Type()))
		v.SetMapIndex(key, val)
	case reflect.Pointer:
		v.Set(reflect.New(v.Type().Elem()))
		fillNonZero(v.Elem())
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).PkgPath == "" {
				fillNonZero(v.Field(i))
			}
		}
	}
}

// TestEveryAppSettingsFieldIsEitherCarriedOrServerOwned is the guard that makes the fix durable:
// adding a field to AppSettings without adding it to the DTO fails here, instead of silently
// becoming the next thing a settings save erases.
func TestEveryAppSettingsFieldIsEitherCarriedOrServerOwned(t *testing.T) {
	var full domain.AppSettings
	fillNonZero(reflect.ValueOf(&full).Elem())

	roundTripped := wails.DTOToAppSettings(wails.AppSettingsToDTO(full))

	typ := reflect.TypeFor[domain.AppSettings]()
	got := reflect.ValueOf(roundTripped)
	for i := 0; i < typ.NumField(); i++ {
		name := typ.Field(i).Name
		survived := !got.Field(i).IsZero()
		switch {
		case survived && serverOwnedSettings[name]:
			t.Errorf("%s is listed as server-owned but the DTO now carries it; either remove it "+
				"from serverOwnedSettings or stop exposing it to the settings dialog", name)
		case !survived && !serverOwnedSettings[name]:
			t.Errorf("AppSettings.%s does not survive the DTO round-trip and is not listed as "+
				"server-owned, so saving settings from the UI erases it", name)
		}
	}
}

type roundTripVaultRepo struct {
	domain.VaultRepository
	data *domain.VaultData
}

func (r *roundTripVaultRepo) GetData() (*domain.VaultData, error) { return r.data, nil }

func (r *roundTripVaultRepo) UpdateData(_ context.Context, mutate func(*domain.VaultData) error) error {
	return mutate(r.data)
}

// The other half: the fields the DTO deliberately drops must actually be carried across the save.
// A user who turned on "require signed plugins" and then changed the theme was silently returned
// to accepting unsigned plugins.
func TestSavingSettingsKeepsPluginTrustAndGrants(t *testing.T) {
	repo := &roundTripVaultRepo{data: domain.NewVaultData()}
	repo.data.Settings.Plugins = domain.PluginSettings{
		RequireSignedPlugins: true,
		TrustedPublisherKeys: []string{"publisher-key"},
		SecretAccessGranted:  map[string]bool{"com.example.granted": true},
		Disabled:             map[string]bool{"com.example.off": true},
	}
	repo.data.Settings.Embed = domain.EmbedSettings{SuspendTcpWhenInactive: true}

	svc := usecase.NewSettingsService(repo, nil, nil)
	fromDialog := domain.AppSettings{Theme: "light", UIScalePercent: 100}
	if err := svc.SaveSettings(context.Background(), fromDialog); err != nil {
		t.Fatalf("save settings: %v", err)
	}

	saved := repo.data.Settings
	if !saved.Plugins.RequireSignedPlugins {
		t.Error("RequireSignedPlugins was cleared by an unrelated settings save")
	}
	if len(saved.Plugins.TrustedPublisherKeys) != 1 {
		t.Error("trusted publisher keys were cleared by an unrelated settings save")
	}
	if !saved.Plugins.SecretAccessGranted["com.example.granted"] {
		t.Error("a granted secret-access consent was cleared by an unrelated settings save")
	}
	if !saved.Plugins.Disabled["com.example.off"] {
		t.Error("a disabled plugin was re-enabled by an unrelated settings save")
	}
	if !saved.Embed.SuspendTcpWhenInactive {
		t.Error("embed settings were cleared by an unrelated settings save")
	}
	if saved.Theme != "light" {
		t.Errorf("the dialog's own change did not land: theme = %q", saved.Theme)
	}
}
