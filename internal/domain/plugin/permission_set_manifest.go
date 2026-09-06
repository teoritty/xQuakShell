package plugin

import (
	"slices"
	"strings"
)

// PermissionSetFromManifest reduces a manifest's capabilities to the set of permissions it asks for.
//
// Token names come from the JSON tags rather than the Go field names, so they match the wire
// contract a plugin author reads. A field carrying values contributes one token per value; a boolean
// contributes a single token when it is on and nothing when it is off.
//
// Two groups of fields deliberately contribute nothing, and the reasoning is in permissionExclusions
// below rather than scattered through these helpers. A test walks every capability field by
// reflection and requires each to be either mapped here or listed there, because a field nobody
// classified is one a plugin can widen without the user being asked.
func PermissionSetFromManifest(m *Manifest) PermissionSet {
	if m == nil {
		return PermissionSet{}
	}
	caps := m.Capabilities
	var tokens []string
	tokens = append(tokens, networkPermissions(caps.Network)...)
	tokens = append(tokens, filesystemPermissions(caps.FS)...)
	tokens = append(tokens, eventPermissions(caps.Events)...)
	tokens = append(tokens, vaultPermissions(caps.Vault)...)
	tokens = append(tokens, sessionPermissions(caps.Session)...)
	tokens = append(tokens, authPermissions(caps.Auth)...)
	tokens = append(tokens, tunnelPermissions(caps.Tunnel)...)
	tokens = append(tokens, channelPermissions(caps.Channel)...)
	tokens = append(tokens, discoveryPermissions(caps.Discovery)...)
	tokens = append(tokens, uiPermissions(caps.UI)...)
	return newPermissionSet(tokens)
}

// permissionExclusions records the capability fields that grant nothing, with the reason for each.
// It is consulted only by the classification test; the value of writing it down is that "decided
// this grants nothing" stays distinguishable from "forgot this field exists".
//
//   - i18n.locales is informational. The host neither validates the list nor withholds the language
//     notification for a tag absent from it, so counting it would raise a permissions dialog every
//     time a plugin shipped a translation.
//   - The numeric ceilings widen what a plugin may consume, not what it may reach, and each is
//     already bounded by a host ceiling. Counting them would turn performance tuning into a consent
//     prompt, which is how a dialog stops being read.
var permissionExclusions = []string{
	"i18n.locales",
	"session.maxTunnelBandwidthKbps",
	"tunnel.maxConcurrentChannels",
	"channel.maxConcurrent",
	"channel.maxThroughputKbps",
	"ui.maxSurfaces",
}

// valuePermissions turns one value-bearing field into one token per value.
func valuePermissions(name string, values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		out = append(out, name+":"+value)
	}
	return out
}

// flagPermission turns a boolean field into a token when it is set. An unset flag asks for nothing
// and must contribute nothing, or every manifest would appear to request every capability.
func flagPermission(name string, on bool) []string {
	if !on {
		return nil
	}
	return []string{name}
}

func networkPermissions(caps *NetworkCaps) []string {
	if caps == nil {
		return nil
	}
	out := valuePermissions("network.outbound", caps.Outbound)
	out = append(out, flagPermission(PermissionArbitraryOutbound, caps.AllowArbitraryOutbound)...)
	return append(out, flagPermission(PermissionPrivateNetworks, caps.AllowPrivateNetworks)...)
}

func filesystemPermissions(caps *FSCaps) []string {
	if caps == nil {
		return nil
	}
	out := valuePermissions("filesystem.read", caps.Read)
	return append(out, valuePermissions("filesystem.write", caps.Write)...)
}

func eventPermissions(caps *EventCaps) []string {
	if caps == nil {
		return nil
	}
	out := valuePermissions("events.subscribe", caps.Subscribe)
	return append(out, valuePermissions("events.publish", caps.Publish)...)
}

func vaultPermissions(caps *VaultCaps) []string {
	if caps == nil {
		return nil
	}
	out := valuePermissions("vault.readConnectionFields", caps.ReadConnectionFields)
	return append(out, valuePermissions(permissionVaultGetSecret, caps.GetSecret)...)
}

func sessionPermissions(caps *SessionCaps) []string {
	if caps == nil {
		return nil
	}
	out := valuePermissions("session.connectProtocols", caps.ConnectProtocols)
	out = append(out, flagPermission("session.terminal", caps.Terminal)...)
	out = append(out, flagPermission("session.embed", caps.Embed)...)
	out = append(out, flagPermission("session.remoteFs", caps.RemoteFS)...)
	return append(out, flagPermission(PermissionMultiSession, caps.AllowMultiSession)...)
}

func authPermissions(caps *AuthCaps) []string {
	if caps == nil {
		return nil
	}
	out := flagPermission(PermissionAuthProvider, caps.Provider)
	return append(out, valuePermissions(permissionAuthMethods, caps.Methods)...)
}

func tunnelPermissions(caps *TunnelCaps) []string {
	if caps == nil {
		return nil
	}
	return flagPermission(PermissionTunnelProvider, caps.Provider)
}

func channelPermissions(caps *ChannelCaps) []string {
	if caps == nil {
		return nil
	}
	out := valuePermissions("channel.purposes", caps.Purposes)
	for _, template := range caps.ExecCommands {
		out = append(out, execCommandPermission(template))
	}
	return out
}

// execCommandPermission names one allowlisted argv template, parameters included.
//
// This is the one place that deliberately over-reports. An argv template is a command the host runs
// on the plugin's behalf, and the substitution parameters decide what it can be made to run, so a
// change to either is a change to what the user agreed the plugin may execute.
func execCommandPermission(template ExecCommandTemplate) string {
	name := permissionExecCommands + ":" + strings.Join(template.Argv, " ")
	if len(template.Params) == 0 {
		return name
	}
	params := make([]string, 0, len(template.Params))
	for key := range template.Params {
		params = append(params, key)
	}
	slices.Sort(params)
	return name + " {" + strings.Join(params, ",") + "}"
}

func discoveryPermissions(caps *DiscoveryCaps) []string {
	if caps == nil {
		return nil
	}
	return valuePermissions("discovery.parentProtocols", caps.ParentProtocols)
}

func uiPermissions(caps *UICaps) []string {
	if caps == nil {
		return nil
	}
	out := valuePermissions("ui.surfaces", caps.Surfaces)
	out = append(out, flagPermission("ui.dialogs", caps.Dialogs)...)
	return append(out, flagPermission("ui.nodeDetails", caps.NodeDetails)...)
}
