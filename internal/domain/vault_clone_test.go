package domain

import "testing"

func TestCloneVaultData_DoesNotShareNestedState(t *testing.T) {
	original := NewVaultData()
	original.Connections = []Connection{{
		ID:   "c1",
		Tags: []string{"prod"},
		Users: []ConnectionUser{{
			ID:      "u1",
			KeyAuth: &KeyAuthConfig{IdentityIDs: []string{"k1"}},
		}},
		JumpChain: JumpChainConfig{Hops: []JumpHop{{
			ID:      "h1",
			KeyAuth: &KeyAuthConfig{IdentityIDs: []string{"jk1"}},
		}}},
	}}
	original.KeyBlobs["k1"] = IdentityBlob{PEMData: []byte("pem")}
	original.Passwords["p1"] = PasswordBlob{Value: []byte("secret"), Label: "label"}
	original.Settings.Plugins.SecretAccessGranted = map[string]bool{"plugin": true}
	original.Settings.Plugins.ArbitraryNetworkAccessGranted = map[string]bool{"net-plugin": true}

	clone := CloneVaultData(original)
	clone.Connections[0].Tags[0] = "changed"
	clone.Connections[0].Users[0].KeyAuth.IdentityIDs[0] = "changed"
	clone.Connections[0].JumpChain.Hops[0].KeyAuth.IdentityIDs[0] = "changed"
	clone.KeyBlobs["k1"] = IdentityBlob{PEMData: []byte("changed")}
	clone.Passwords["p1"] = PasswordBlob{Value: []byte("changed")}
	clone.Settings.Plugins.SecretAccessGranted["plugin"] = false
	clone.Settings.Plugins.ArbitraryNetworkAccessGranted["net-plugin"] = false

	if original.Connections[0].Tags[0] != "prod" {
		t.Fatal("connection tags shared")
	}
	if original.Connections[0].Users[0].KeyAuth.IdentityIDs[0] != "k1" {
		t.Fatal("user key auth shared")
	}
	if original.Connections[0].JumpChain.Hops[0].KeyAuth.IdentityIDs[0] != "jk1" {
		t.Fatal("hop key auth shared")
	}
	if string(original.KeyBlobs["k1"].PEMData) != "pem" {
		t.Fatal("key blob shared")
	}
	if string(original.Passwords["p1"].Value) != "secret" {
		t.Fatal("password shared")
	}
	if !original.Settings.Plugins.SecretAccessGranted["plugin"] {
		t.Fatal("plugin settings map shared")
	}
	if !original.Settings.Plugins.ArbitraryNetworkAccessGranted["net-plugin"] {
		t.Fatal("arbitrary network grant map shared")
	}
}

func TestCloneVaultData_NilInput(t *testing.T) {
	if CloneVaultData(nil) != nil {
		t.Fatal("nil input should return nil")
	}
}

// A snapshot that shared the key map with the live vault would let a write through the snapshot
// change what seals the user's replicas. Every other map here is deep-copied; this one was added
// later and is exactly the kind that gets missed.
func TestCloningDoesNotShareTheReplicaKeys(t *testing.T) {
	in := NewVaultData()
	in.ReplicaKeys["com.example.sync"] = "ORIGINAL"

	out := CloneVaultData(in)
	out.ReplicaKeys["com.example.sync"] = "REPLACED"

	if in.ReplicaKeys["com.example.sync"] != "ORIGINAL" {
		t.Fatal("writing to the clone changed the original's replication key")
	}
}
