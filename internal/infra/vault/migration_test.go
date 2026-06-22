package vault_test

import (
	"encoding/json"
	"strings"
	"testing"

	"ssh-client/internal/domain"
	"ssh-client/internal/infra/vault"
)

func TestMigrateV2ToV3StripsVPNData(t *testing.T) {
	legacyJSON := `{
		"version": 2,
		"folders": [],
		"connections": [{
			"id": "c1",
			"name": "VPN host",
			"host": "10.0.0.1",
			"port": 22,
			"vpnProfileId": "vpn-123"
		}],
		"identities": {},
		"keyBlobs": {},
		"knownHosts": [],
		"passwords": {},
		"vpnProfiles": {
			"vpn-123": {"id": "vpn-123", "label": "wg", "protocol": "wireguard"}
		},
		"settings": {"lockout": {"enabled": false, "idleTimeout": 0, "lockOnMinimize": false}}
	}`

	data, err := vault.UnmarshalLegacyVault([]byte(legacyJSON))
	if err != nil {
		t.Fatalf("unmarshal legacy: %v", err)
	}
	if data.Version != 2 {
		t.Fatalf("expected version 2 before migration, got %d", data.Version)
	}

	vault.MigrateVaultData(data)

	if data.Version != domain.CurrentVaultVersion {
		t.Errorf("Version: got %d, want %d", data.Version, domain.CurrentVaultVersion)
	}
	if len(data.Connections) != 1 {
		t.Fatalf("expected 1 connection, got %d", len(data.Connections))
	}

	out, err := json.Marshal(data)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	outStr := string(out)
	if strings.Contains(outStr, "vpnProfileId") || strings.Contains(outStr, "vpnProfiles") {
		t.Errorf("migrated vault should not contain VPN fields, got: %s", outStr)
	}
}
