package domain

// CloneVaultData returns a deep copy of vault data suitable for snapshots.
// Nil input returns nil.
func CloneVaultData(in *VaultData) *VaultData {
	if in == nil {
		return nil
	}
	out := &VaultData{
		Version:     in.Version,
		Folders:     cloneFolders(in.Folders),
		Connections: cloneConnections(in.Connections),
		Identities:  cloneIdentities(in.Identities),
		KeyBlobs:    cloneKeyBlobs(in.KeyBlobs),
		KnownHosts:  cloneStrings(in.KnownHosts),
		Passwords:   clonePasswords(in.Passwords),
		Settings:    CloneAppSettings(in.Settings),
	}
	return out
}

func cloneFolders(in []ConnectionFolder) []ConnectionFolder {
	if in == nil {
		return nil
	}
	out := make([]ConnectionFolder, len(in))
	copy(out, in)
	return out
}

func cloneConnections(in []Connection) []Connection {
	if in == nil {
		return nil
	}
	out := make([]Connection, len(in))
	for i := range in {
		out[i] = CloneConnection(in[i])
	}
	return out
}

// CloneConnection returns a deep copy of a connection.
func CloneConnection(in Connection) Connection {
	out := in
	out.User = in.User
	out.IdentityIDs = cloneStrings(in.IdentityIDs)
	out.Users = cloneConnectionUsers(in.Users)
	out.Tags = cloneStrings(in.Tags)
	out.JumpChain = cloneJumpChain(in.JumpChain)
	return out
}

func cloneConnectionUsers(in []ConnectionUser) []ConnectionUser {
	if in == nil {
		return nil
	}
	out := make([]ConnectionUser, len(in))
	for i := range in {
		out[i] = CloneConnectionUser(in[i])
	}
	return out
}

// CloneConnectionUser returns a deep copy of a connection user.
func CloneConnectionUser(in ConnectionUser) ConnectionUser {
	out := in
	if in.KeyAuth != nil {
		ka := *in.KeyAuth
		ka.IdentityIDs = cloneStrings(in.KeyAuth.IdentityIDs)
		out.KeyAuth = &ka
	}
	if in.PassAuth != nil {
		pa := *in.PassAuth
		out.PassAuth = &pa
	}
	return out
}

func cloneJumpChain(in JumpChainConfig) JumpChainConfig {
	if len(in.Hops) == 0 {
		return JumpChainConfig{}
	}
	out := JumpChainConfig{Hops: make([]JumpHop, len(in.Hops))}
	for i := range in.Hops {
		out.Hops[i] = CloneJumpHop(in.Hops[i])
	}
	return out
}

// CloneJumpHop returns a deep copy of a jump hop.
func CloneJumpHop(in JumpHop) JumpHop {
	out := in
	if in.KeyAuth != nil {
		ka := *in.KeyAuth
		ka.IdentityIDs = cloneStrings(in.KeyAuth.IdentityIDs)
		out.KeyAuth = &ka
	}
	if in.PassAuth != nil {
		pa := *in.PassAuth
		out.PassAuth = &pa
	}
	return out
}

func cloneIdentities(in map[string]SSHIdentity) map[string]SSHIdentity {
	if in == nil {
		return nil
	}
	out := make(map[string]SSHIdentity, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

func cloneKeyBlobs(in map[string]IdentityBlob) map[string]IdentityBlob {
	if in == nil {
		return nil
	}
	out := make(map[string]IdentityBlob, len(in))
	for k, v := range in {
		out[k] = IdentityBlob{PEMData: cloneBytes(v.PEMData)}
	}
	return out
}

func clonePasswords(in map[string]PasswordBlob) map[string]PasswordBlob {
	if in == nil {
		return nil
	}
	out := make(map[string]PasswordBlob, len(in))
	for k, v := range in {
		out[k] = PasswordBlob{
			Value: cloneBytes(v.Value),
			Label: v.Label,
		}
	}
	return out
}

// CloneAppSettings returns a deep copy of application settings.
func CloneAppSettings(in *AppSettings) *AppSettings {
	if in == nil {
		return nil
	}
	out := *in
	out.Lockout = in.Lockout
	out.Terminal = in.Terminal
	out.Ping = in.Ping
	out.Transfer = in.Transfer
	out.SessionHotkeys = in.SessionHotkeys
	out.AuditLog = in.AuditLog
	out.Plugins = clonePluginSettings(in.Plugins)
	return &out
}

func clonePluginSettings(in PluginSettings) PluginSettings {
	out := in
	out.TrustedPublisherKeys = cloneStrings(in.TrustedPublisherKeys)
	out.SecretAccessGranted = cloneBoolMap(in.SecretAccessGranted)
	out.MultiSessionAccessGranted = cloneBoolMap(in.MultiSessionAccessGranted)
	out.Disabled = cloneBoolMap(in.Disabled)
	return out
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}
	out := make([]string, len(in))
	copy(out, in)
	return out
}

func cloneBytes(in []byte) []byte {
	if in == nil {
		return nil
	}
	return append([]byte(nil), in...)
}

func cloneBoolMap(in map[string]bool) map[string]bool {
	if in == nil {
		return nil
	}
	out := make(map[string]bool, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}
