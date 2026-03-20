package unit

import (
	"testing"

	"ssh-client/internal/infra/guacamole"
)

func TestGenerateEncryptionKey(t *testing.T) {
	key1, err := guacamole.GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey: %v", err)
	}
	if len(key1) != 32 {
		t.Fatalf("key length = %d, want 32", len(key1))
	}

	key2, err := guacamole.GenerateEncryptionKey()
	if err != nil {
		t.Fatalf("GenerateEncryptionKey (second): %v", err)
	}

	same := true
	for i := range key1 {
		if key1[i] != key2[i] {
			same = false
			break
		}
	}
	if same {
		t.Error("two generated keys should not be identical")
	}
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	key, _ := guacamole.GenerateEncryptionKey()

	data := map[string]interface{}{
		"hostname": "192.168.1.100",
		"port":     float64(3389),
		"username": "admin",
		"password": "s3cret!@#",
		"domain":   "CORP",
	}

	token, err := guacamole.EncryptToken(key, data)
	if err != nil {
		t.Fatalf("EncryptToken: %v", err)
	}
	if token == "" {
		t.Fatal("token is empty")
	}

	result, err := guacamole.DecryptToken(key, token)
	if err != nil {
		t.Fatalf("DecryptToken: %v", err)
	}

	for k, expected := range data {
		got, ok := result[k]
		if !ok {
			t.Errorf("key %q missing from decrypted data", k)
			continue
		}
		if got != expected {
			t.Errorf("key %q: got %v, want %v", k, got, expected)
		}
	}
}

func TestDecryptWithWrongKey(t *testing.T) {
	key1, _ := guacamole.GenerateEncryptionKey()
	key2, _ := guacamole.GenerateEncryptionKey()

	data := map[string]interface{}{
		"hostname": "test",
	}

	token, err := guacamole.EncryptToken(key1, data)
	if err != nil {
		t.Fatalf("EncryptToken: %v", err)
	}

	_, err = guacamole.DecryptToken(key2, token)
	if err == nil {
		t.Error("expected error when decrypting with wrong key")
	}
}

func TestDecryptInvalidToken(t *testing.T) {
	key, _ := guacamole.GenerateEncryptionKey()

	tests := []struct {
		name  string
		token string
	}{
		{"empty", ""},
		{"not base64", "!!!invalid!!!"},
		{"valid base64 but not json", "aGVsbG8gd29ybGQ="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := guacamole.DecryptToken(key, tt.token)
			if err == nil {
				t.Error("expected error for invalid token")
			}
		})
	}
}

func TestEncryptEmptyData(t *testing.T) {
	key, _ := guacamole.GenerateEncryptionKey()

	data := map[string]interface{}{}
	token, err := guacamole.EncryptToken(key, data)
	if err != nil {
		t.Fatalf("EncryptToken with empty data: %v", err)
	}

	result, err := guacamole.DecryptToken(key, token)
	if err != nil {
		t.Fatalf("DecryptToken: %v", err)
	}
	if len(result) != 0 {
		t.Errorf("expected empty map, got %v", result)
	}
}

func TestTokenDeterministicPayload(t *testing.T) {
	key, _ := guacamole.GenerateEncryptionKey()
	data := map[string]interface{}{"test": "value"}

	token1, _ := guacamole.EncryptToken(key, data)
	token2, _ := guacamole.EncryptToken(key, data)

	if token1 == token2 {
		t.Error("tokens should differ due to random IV")
	}

	result1, _ := guacamole.DecryptToken(key, token1)
	result2, _ := guacamole.DecryptToken(key, token2)

	if result1["test"] != result2["test"] {
		t.Errorf("decrypted values differ: %v vs %v", result1["test"], result2["test"])
	}
}
