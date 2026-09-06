package plugin_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"xquakshell/internal/domain"
	domainplugin "xquakshell/internal/domain/plugin"
	"xquakshell/internal/usecase"
)

// scopeConnRepo is a tree with one scope folder and one folder outside it:
//
//	sync/          <- the scope root of com.test.vault
//	  prod/        <- holds connection "c1"
//	personal/      <- holds connection "c2"
type scopeConnRepo struct {
	vaultConnRepo
}

func (scopeConnRepo) GetAllFolders(context.Context) ([]domain.ConnectionFolder, error) {
	return []domain.ConnectionFolder{
		{ID: "sync", Name: "Sync"},
		{ID: "prod", Name: "Prod", ParentID: "sync"},
		{ID: "personal", Name: "Personal"},
	}, nil
}

func (r scopeConnRepo) GetByID(_ context.Context, id string) (*domain.Connection, error) {
	folder := "prod"
	if id == "c2" {
		folder = "personal"
	}
	return &domain.Connection{
		ID: id, FolderID: folder,
		Users: []domain.ConnectionUser{{
			ID: "u1", Username: "u", Auth: domain.AuthMethodPassword,
			PassAuth: &domain.PasswordAuthConfig{VaultRef: "p1"},
		}},
		DefaultUserID: "u1",
	}, nil
}

type scopeSettings struct {
	scopeFolder string
	scopePlugin string
}

func (s scopeSettings) PluginSettings() (domain.PluginSettings, error) {
	settings := domain.PluginSettings{}
	settings.RecordGrant(domain.PluginGrant{
		PluginID: "com.test.vault",
		Granted:  []string{domainplugin.PermissionSecretField("password")},
	})
	if s.scopeFolder != "" {
		settings.ScopeRoots = []domain.ScopeRoot{{FolderID: s.scopeFolder, PluginID: s.scopePlugin}}
	}
	return settings, nil
}

type lockState struct{ unlocked bool }

func (l lockState) IsUnlocked() bool { return l.unlocked }

// scopeInbound builds a gate with NO session binding at all, so the only thing that can allow a
// request is the scope.
func scopeInbound(t *testing.T, settings scopeSettings, lock usecase.VaultLockState) *usecase.PluginVaultInbound {
	t.Helper()
	registry := usecase.NewPluginRegistry()
	mustRegister(t, registry, domainplugin.InstalledPlugin{
		Manifest: domainplugin.Manifest{
			ID: "com.test.vault", Name: "V", Version: "1",
			Engine: domainplugin.EngineConfig{Type: domainplugin.EngineGoBinary, Entry: "p.exe"},
			Capabilities: domainplugin.CapabilitySet{
				Vault: &domainplugin.VaultCaps{GetSecret: []string{"password"}},
			},
		},
	})
	inbound := usecase.NewPluginVaultInbound(
		registry, scopeConnRepo{}, vaultPasswordRepo{}, vaultIdentRepo{}, settings, nil,
	)
	inbound.SetAuthorizer(vaultAuthorizer{owns: map[string]bool{}})
	inbound.SetAuditLogger(&recordingVaultAudit{})
	if lock != nil {
		inbound.SetLockState(lock)
	}
	return inbound
}

func scopeGetSecret(inbound *usecase.PluginVaultInbound, connectionID string) error {
	params, _ := json.Marshal(map[string]string{"connectionId": connectionID, "field": "password"})
	_, err := inbound.GetSecret(context.Background(), "com.test.vault", params)
	return err
}

// The second authorization anchor. A synchronisation plugin holds no session - it is a background
// job - so before this there was nothing it could be allowed on. What allows it now is the user
// having put the connection in its folder.
func TestAConnectionInTheScopeIsReachableWithoutASession(t *testing.T) {
	inbound := scopeInbound(t, scopeSettings{scopeFolder: "sync", scopePlugin: "com.test.vault"}, lockState{unlocked: true})

	if err := scopeGetSecret(inbound, "c1"); err != nil {
		t.Fatalf("a connection inside the plugin's scope was refused: %v", err)
	}
}

// The clause that carries ADR-022 decision 6, and the reason the scope anchor is not a hole.
//
// A scope is a position in a folder tree: it survives a lock without any effort at all. Today a
// plugin is powerless once the vault closes only because its sessions are gone and the vault refuses
// to be read. An anchor that did not check would therefore GRANT access after the lock that no
// plugin has today - through the mechanism added to constrain them.
func TestTheScopeAnchorIsVoidWhileTheVaultIsLocked(t *testing.T) {
	inbound := scopeInbound(t, scopeSettings{scopeFolder: "sync", scopePlugin: "com.test.vault"}, lockState{unlocked: false})

	if err := scopeGetSecret(inbound, "c1"); !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("err = %v, want ErrCapabilityDenied while the vault is locked", err)
	}
}

// Composition can leave the lock state unwired. With no way to tell whether the vault is open, the
// scope anchor must refuse rather than assume.
func TestTheScopeAnchorRefusesWhenLockStateIsUnknown(t *testing.T) {
	inbound := scopeInbound(t, scopeSettings{scopeFolder: "sync", scopePlugin: "com.test.vault"}, nil)

	if err := scopeGetSecret(inbound, "c1"); !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("err = %v, want ErrCapabilityDenied when the lock state is unknown", err)
	}
}

// Default-deny is the whole point of the folder. A connection the user kept outside it does not
// leave the machine, and "outside" includes a folder sitting right beside the scope.
func TestAConnectionOutsideTheScopeIsRefused(t *testing.T) {
	inbound := scopeInbound(t, scopeSettings{scopeFolder: "sync", scopePlugin: "com.test.vault"}, lockState{unlocked: true})

	if err := scopeGetSecret(inbound, "c2"); !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("err = %v, want ErrCapabilityDenied for a connection outside the scope", err)
	}
}

// A scope belonging to another plugin is not this plugin's. Two sync plugins side by side must not
// read each other's connections merely because both have a folder.
func TestAnotherPluginsScopeDoesNotAllowThisOne(t *testing.T) {
	inbound := scopeInbound(t, scopeSettings{scopeFolder: "sync", scopePlugin: "com.example.other"}, lockState{unlocked: true})

	if err := scopeGetSecret(inbound, "c1"); !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("err = %v, want ErrCapabilityDenied for another plugin's scope", err)
	}
}

// With no scope declared at all, nothing changes: a plugin with no session reaches nothing, which
// is what every vault looks like until a plugin that wants a scope is installed.
func TestWithoutAScopeAPluginWithNoSessionReachesNothing(t *testing.T) {
	inbound := scopeInbound(t, scopeSettings{}, lockState{unlocked: true})

	if err := scopeGetSecret(inbound, "c1"); !errors.Is(err, domainplugin.ErrCapabilityDenied) {
		t.Fatalf("err = %v, want ErrCapabilityDenied with no scope declared", err)
	}
}

// The anchor that existed before this one still works, and still works on its own. A plugin holding
// a live session for a connection reaches it whether or not any scope exists.
func TestASessionStillAllowsAConnectionWithNoScope(t *testing.T) {
	inbound := scopeInbound(t, scopeSettings{}, lockState{unlocked: true})
	inbound.SetAuthorizer(vaultAuthorizer{owns: map[string]bool{"com.test.vault:c2": true}})

	if err := scopeGetSecret(inbound, "c2"); err != nil {
		t.Fatalf("a session-owned connection was refused: %v", err)
	}
}
