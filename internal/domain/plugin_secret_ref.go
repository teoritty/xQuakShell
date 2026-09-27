package domain

const pluginSecretRefPrefix = "secret:"

// PluginSecretRef names the vault entry that holds one secret plugin field of one connection.
//
// The connection id is part of the name, so the reference belongs to exactly one connection: two
// connections that shared one would share the secret, and clearing the field on either would
// delete it for both.
func PluginSecretRef(connID, fieldID string) string {
	return pluginSecretRefPrefix + connID + "." + fieldID
}
