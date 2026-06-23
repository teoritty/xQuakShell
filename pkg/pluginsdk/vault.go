package pluginsdk

import "encoding/base64"

// VaultConnectionParams requests connection metadata.
type VaultConnectionParams struct {
	ConnectionID string `json:"connectionId"`
}

// VaultSecretParams requests a secret field for a connection.
type VaultSecretParams struct {
	ConnectionID string `json:"connectionId"`
	Field        string `json:"field"`
}

// VaultSecretResult is the vault.getSecret response payload.
type VaultSecretResult struct {
	Field       string `json:"field"`
	ValueBase64 string `json:"valueBase64"`
}

// GetConnection fetches allowed connection metadata via vault.getConnection.
func (c *Client) GetConnection(connectionID string, out any) error {
	return c.CallCore("vault.getConnection", VaultConnectionParams{ConnectionID: connectionID}, out)
}

// GetSecret fetches an allowed secret field via vault.getSecret.
func (c *Client) GetSecret(connectionID, field string) ([]byte, error) {
	var res VaultSecretResult
	if err := c.CallCore("vault.getSecret", VaultSecretParams{
		ConnectionID: connectionID,
		Field:        field,
	}, &res); err != nil {
		return nil, err
	}
	if res.ValueBase64 == "" {
		return nil, nil
	}
	return base64.StdEncoding.DecodeString(res.ValueBase64)
}
