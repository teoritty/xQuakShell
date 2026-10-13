package synctrust_test

import (
	"encoding/json"
	"testing"
	"time"

	"xquakshell/internal/domain"
	"xquakshell/internal/domain/agentforward"
	"xquakshell/internal/domain/synctrust"
	"xquakshell/internal/domain/vaultsync"
)

func raw(t *testing.T, v any) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func section(t *testing.T, s domain.AppSettings, name string) json.RawMessage {
	t.Helper()
	sections, err := synctrust.SplitSettings(s)
	if err != nil {
		t.Fatal(err)
	}
	return sections[name]
}

type trustCase struct {
	ty              synctrust.RecordType
	local, incoming json.RawMessage
	want            synctrust.Action
}

func trustCases(t *testing.T) map[string]trustCase {
	base := *domain.NewVaultData().Settings
	with := func(f func(*domain.AppSettings)) domain.AppSettings { s := base; f(&s); return s }
	lock := func(enabled bool, idle time.Duration, minimize bool) domain.AppSettings {
		return with(func(s *domain.AppSettings) {
			s.Lockout = domain.LockoutSettings{Enabled: enabled, IdleTimeout: idle, LockOnMinimize: minimize}
		})
	}
	lockout := synctrust.SettingsType("lockout")
	plugins := synctrust.SettingsType("plugins")
	ident := func(f func(*domain.SSHIdentity)) json.RawMessage {
		i := domain.SSHIdentity{ID: "k"}
		f(&i)
		return raw(t, map[string]any{"identity": i})
	}
	conn := func(af *agentforward.Config) json.RawMessage {
		return raw(t, domain.Connection{ID: "c", Port: 22, AgentForwarding: af})
	}
	host := func(line string) json.RawMessage { return raw(t, map[string]string{"line": line}) }
	peer := func(m byte) json.RawMessage {
		return raw(t, domain.PeerTrustEntry{Scope: "p", Subject: "s", Material: []byte{m}})
	}
	return map[string]trustCase{
		"plugins unchanged":        {plugins, section(t, base, "plugins"), section(t, base, "plugins"), synctrust.ActionApply},
		"plugins unsigned allowed": {plugins, section(t, base, "plugins"), section(t, with(func(s *domain.AppSettings) { s.Plugins.RequireSignedPlugins = false }), "plugins"), synctrust.ActionPending},
		"plugins new trusted key": {plugins, section(t, base, "plugins"),
			section(t, with(func(s *domain.AppSettings) { s.Plugins.TrustedPublisherKeys = []string{"k"} }), "plugins"), synctrust.ActionPending},
		"plugins grant widened": {plugins, section(t, base, "plugins"),
			section(t, with(func(s *domain.AppSettings) { s.Plugins.SecretAccessGranted = map[string]bool{"p": true} }), "plugins"), synctrust.ActionPending},
		"plugins grant revoked": {plugins,
			section(t, with(func(s *domain.AppSettings) { s.Plugins.SecretAccessGranted = map[string]bool{"p": true} }), "plugins"),
			section(t, base, "plugins"), synctrust.ActionApply},
		"plugins first sight weakens default": {plugins, nil,
			section(t, with(func(s *domain.AppSettings) { s.Plugins.RequireSignedPlugins = false }), "plugins"), synctrust.ActionPending},

		"lockout shorter":           {lockout, section(t, lock(true, 10*time.Minute, false), "lockout"), section(t, lock(true, 5*time.Minute, false), "lockout"), synctrust.ActionApply},
		"lockout same":              {lockout, section(t, lock(true, 5*time.Minute, false), "lockout"), section(t, lock(true, 5*time.Minute, false), "lockout"), synctrust.ActionApply},
		"lockout longer":            {lockout, section(t, lock(true, 5*time.Minute, false), "lockout"), section(t, lock(true, 5*time.Minute+1, false), "lockout"), synctrust.ActionPending},
		"lockout disabled":          {lockout, section(t, lock(true, 5*time.Minute, false), "lockout"), section(t, lock(false, 5*time.Minute, false), "lockout"), synctrust.ActionPending},
		"lockout enabled":           {lockout, section(t, lock(false, 5*time.Minute, false), "lockout"), section(t, lock(true, time.Hour, false), "lockout"), synctrust.ActionApply},
		"lockout zero idle":         {lockout, section(t, lock(true, 5*time.Minute, false), "lockout"), section(t, lock(true, 0, false), "lockout"), synctrust.ActionPending},
		"lockout minimize dropped":  {lockout, section(t, lock(true, 5*time.Minute, true), "lockout"), section(t, lock(true, 5*time.Minute, false), "lockout"), synctrust.ActionPending},
		"lockout minimize added":    {lockout, section(t, lock(true, 5*time.Minute, false), "lockout"), section(t, lock(true, 5*time.Minute, true), "lockout"), synctrust.ActionApply},
		"lockout first sight":       {lockout, nil, section(t, lock(false, 0, false), "lockout"), synctrust.ActionApply},
		"lockout disabled, longer":  {lockout, section(t, lock(false, 5*time.Minute, false), "lockout"), section(t, lock(false, time.Hour, false), "lockout"), synctrust.ActionApply},
		"lockout settings deletion": {lockout, section(t, base, "lockout"), nil, synctrust.ActionRefuse},

		"identity allow plugins":        {synctrust.TypeIdentity, ident(func(*domain.SSHIdentity) {}), ident(func(i *domain.SSHIdentity) { i.AllowPlugins = true }), synctrust.ActionPending},
		"identity allow forwarding":     {synctrust.TypeIdentity, ident(func(*domain.SSHIdentity) {}), ident(func(i *domain.SSHIdentity) { i.AllowForwarding = true }), synctrust.ActionPending},
		"identity revoke plugins":       {synctrust.TypeIdentity, ident(func(i *domain.SSHIdentity) { i.AllowPlugins = true }), ident(func(*domain.SSHIdentity) {}), synctrust.ActionApply},
		"identity exportable again":     {synctrust.TypeIdentity, ident(func(i *domain.SSHIdentity) { i.NonExportable = true }), ident(func(*domain.SSHIdentity) {}), synctrust.ActionRefuse},
		"identity made non-exportable":  {synctrust.TypeIdentity, ident(func(*domain.SSHIdentity) {}), ident(func(i *domain.SSHIdentity) { i.NonExportable = true }), synctrust.ActionApply},
		"identity comment":              {synctrust.TypeIdentity, ident(func(*domain.SSHIdentity) {}), ident(func(i *domain.SSHIdentity) { i.Comment = "x" }), synctrust.ActionApply},
		"identity new":                  {synctrust.TypeIdentity, nil, ident(func(i *domain.SSHIdentity) { i.AllowPlugins = true }), synctrust.ActionApply},
		"identity deleted":              {synctrust.TypeIdentity, ident(func(i *domain.SSHIdentity) { i.NonExportable = true }), nil, synctrust.ActionApply},
		"connection forwarding on":      {synctrust.TypeConnection, conn(nil), conn(&agentforward.Config{Enabled: true, IdentityIDs: []string{"k"}}), synctrust.ActionPending},
		"connection forwarding new":     {synctrust.TypeConnection, nil, conn(&agentforward.Config{Enabled: true, IdentityIDs: []string{"k"}}), synctrust.ActionPending},
		"connection forwarding grows":   {synctrust.TypeConnection, conn(&agentforward.Config{Enabled: true, IdentityIDs: []string{"k"}}), conn(&agentforward.Config{Enabled: true, IdentityIDs: []string{"k", "j"}}), synctrust.ActionPending},
		"connection forwarding shrinks": {synctrust.TypeConnection, conn(&agentforward.Config{Enabled: true, IdentityIDs: []string{"k", "j"}}), conn(&agentforward.Config{Enabled: true, IdentityIDs: []string{"j"}}), synctrust.ActionApply},
		"connection forwarding off":     {synctrust.TypeConnection, conn(&agentforward.Config{Enabled: true, IdentityIDs: []string{"k"}}), conn(nil), synctrust.ActionApply},
		"connection disabled grows":     {synctrust.TypeConnection, conn(&agentforward.Config{IdentityIDs: []string{"k"}}), conn(&agentforward.Config{IdentityIDs: []string{"k", "j"}}), synctrust.ActionApply},
		"connection plain new":          {synctrust.TypeConnection, nil, conn(nil), synctrust.ActionApply},
		"known host new":                {synctrust.TypeKnownHost, nil, host("h ssh-rsa A"), synctrust.ActionApply},
		"known host replaced":           {synctrust.TypeKnownHost, host("h ssh-rsa A"), host("h ssh-rsa B"), synctrust.ActionPending},
		"known host unchanged":          {synctrust.TypeKnownHost, host("h ssh-rsa A"), host("h ssh-rsa A"), synctrust.ActionApply},
		"known host removed":            {synctrust.TypeKnownHost, host("h ssh-rsa A"), nil, synctrust.ActionApply},
		"peer trust new":                {synctrust.TypePeerTrust, nil, peer(1), synctrust.ActionApply},
		"peer trust re-added": {synctrust.TypePeerTrust, peer(1),
			raw(t, domain.PeerTrustEntry{Scope: "p", Subject: "s", Material: []byte{1}, AddedAt: time.Unix(5, 0)}), synctrust.ActionApply},
		"peer trust replaced": {synctrust.TypePeerTrust, peer(1), peer(2), synctrust.ActionPending},
		"folder":              {synctrust.TypeFolder, nil, raw(t, domain.ConnectionFolder{ID: "f", Name: "x"}), synctrust.ActionApply},
	}
}

func TestClassify(t *testing.T) {
	for name, c := range trustCases(t) {
		v, err := synctrust.Classify(c.ty, c.local, c.incoming)
		if err != nil {
			t.Errorf("%s: %v", name, err)
			continue
		}
		if v.Action != c.want {
			t.Errorf("%s: action %v, want %v", name, v.Action, c.want)
		}
		if v.Action != synctrust.ActionApply && v.Reason == "" {
			t.Errorf("%s: a held or refused change carries no reason to show the user", name)
		}
	}
}

func TestClassifyRefusesUndecodableData(t *testing.T) {
	if _, err := synctrust.Classify(synctrust.TypeIdentity, nil, json.RawMessage(`{"identity":{"id":1}}`)); err == nil {
		t.Fatal("an undecodable identity was classified; it must be quarantined instead")
	}
}

func TestFieldPolicy(t *testing.T) {
	cred := vaultsync.FieldCredential
	for _, c := range []struct {
		ty   synctrust.RecordType
		path []string
		want vaultsync.FieldClass
	}{
		{synctrust.TypeConnection, []string{"host"}, cred},
		{synctrust.TypeConnection, []string{"port"}, cred},
		{synctrust.TypeConnection, []string{"jumpChain", "hops"}, cred},
		{synctrust.TypeConnection, []string{"agentForwarding", "enabled"}, cred},
		{synctrust.TypeConnection, []string{"users", "[id=u1]", "passAuth", "vaultRef"}, cred},
		{synctrust.TypeConnection, []string{"users", "[id=u1]", "authMethod"}, cred},
		{synctrust.TypeConnection, []string{"users", "[id=u1]", "keyAuth"}, cred},
		{synctrust.TypeConnection, []string{"users", "[id=u1]", "pluginAuth"}, cred},
		{synctrust.TypeConnection, []string{"users", "[id=u1]", "label"}, vaultsync.FieldOrdinary},
		{synctrust.TypeConnection, []string{"name"}, vaultsync.FieldOrdinary},
		{synctrust.TypeIdentity, []string{"blob", "pemData"}, cred},
		{synctrust.TypeIdentity, []string{"identity", "policy"}, cred},
		{synctrust.TypeIdentity, []string{"identity", "allowPlugins"}, cred},
		{synctrust.TypeIdentity, []string{"identity", "allowForwarding"}, cred},
		{synctrust.TypeIdentity, []string{"identity", "nonExportable"}, cred},
		{synctrust.TypeIdentity, []string{"identity", "comment"}, vaultsync.FieldOrdinary},
		{synctrust.TypePassword, []string{"value"}, cred},
		{synctrust.TypePassword, []string{"label"}, vaultsync.FieldOrdinary},
		{synctrust.TypePluginSecret, []string{"value"}, cred},
		{synctrust.TypeKnownHost, []string{"line"}, cred},
		{synctrust.TypePeerTrust, []string{"material"}, cred},
		{synctrust.TypeFolder, []string{"name"}, vaultsync.FieldOrdinary},
		{synctrust.SettingsType("terminal"), []string{"fontSize"}, vaultsync.FieldOrdinary},
		{synctrust.TypeConnection, nil, vaultsync.FieldOrdinary},
	} {
		if got := synctrust.FieldPolicyFor(c.ty)(c.path); got != c.want {
			t.Errorf("%s %v: class %v, want %v", c.ty, c.path, got, c.want)
		}
	}
}
