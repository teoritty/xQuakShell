package domain

// CloneVaultData returns a deep copy of vault data suitable for snapshots.
// Nil input returns nil.
func CloneVaultData(in *VaultData) *VaultData {
	if in == nil {
		return nil
	}
	out := &VaultData{
		Version:       in.Version,
		Folders:       cloneFolders(in.Folders),
		Connections:   cloneConnections(in.Connections),
		Identities:    cloneIdentities(in.Identities),
		KeyBlobs:      cloneKeyBlobs(in.KeyBlobs),
		KnownHosts:    cloneStrings(in.KnownHosts),
		Passwords:     clonePasswords(in.Passwords),
		PluginSecrets: clonePluginSecrets(in.PluginSecrets),
		PeerTrust:     clonePeerTrust(in.PeerTrust),
		Settings:      CloneAppSettings(in.Settings),
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
	out.Users = cloneConnectionUsers(in.Users)
	out.Tags = cloneStrings(in.Tags)
	out.JumpChain = cloneJumpChain(in.JumpChain)
	if in.PluginFields != nil {
		out.PluginFields = make(map[string]string, len(in.PluginFields))
		for k, v := range in.PluginFields {
			out.PluginFields[k] = v
		}
	}
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

// cloneKeyBlobs copies every field of a blob, not only the key bytes.
//
// Listing them one by one is what makes a forgotten field a compile-time-visible omission rather
// than silent data loss: every write goes through a clone, so a dropped DataKey would not fail
// anything at the time — it would quietly persist a vault whose keys can never be opened again.
func cloneKeyBlobs(in map[string]IdentityBlob) map[string]IdentityBlob {
	if in == nil {
		return nil
	}
	out := make(map[string]IdentityBlob, len(in))
	for k, v := range in {
		out[k] = IdentityBlob{
			PEMData: cloneBytes(v.PEMData),
			DataKey: cloneBytes(v.DataKey),
			Legacy:  v.Legacy,
		}
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

func clonePluginSecrets(in map[string][]byte) map[string][]byte {
	if in == nil {
		return nil
	}
	out := make(map[string][]byte, len(in))
	for k, v := range in {
		out[k] = cloneBytes(v)
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
	out.AuthProviderAccessGranted = cloneBoolMap(in.AuthProviderAccessGranted)
	out.TunnelProviderAccessGranted = cloneBoolMap(in.TunnelProviderAccessGranted)
	out.MultiSessionAccessGranted = cloneBoolMap(in.MultiSessionAccessGranted)
	out.ArbitraryNetworkAccessGranted = cloneBoolMap(in.ArbitraryNetworkAccessGranted)
	out.Disabled = cloneBoolMap(in.Disabled)
	out.PluginGrants = clonePluginGrants(in.PluginGrants)
	return out
}

func clonePluginGrants(in []PluginGrant) []PluginGrant {
	if in == nil {
		return nil
	}
	out := make([]PluginGrant, len(in))
	for i, grant := range in {
		out[i] = cloneGrant(grant)
	}
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

// clonePeerTrust copies trust entries together with their material.
//
// The material is copied rather than reused: a slice in the clone sharing its array with the
// original would let an edit of the clone silently change stored trust.
func clonePeerTrust(in []PeerTrustEntry) []PeerTrustEntry {
	if in == nil {
		return nil
	}
	out := make([]PeerTrustEntry, len(in))
	for i := range in {
		out[i] = in[i]
		out[i].Material = append([]byte(nil), in[i].Material...)
	}
	return out
}
