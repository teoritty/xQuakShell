package guacamole

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
)

// EncryptToken encrypts a JSON-serializable value with AES-256-CBC.
// Output format: base64(JSON({"iv": base64(iv), "value": base64(ciphertext)}))
// Compatible with the guacamole-lite Crypt.js token format.
func EncryptToken(key []byte, data interface{}) (string, error) {
	plaintext, err := json.Marshal(data)
	if err != nil {
		return "", fmt.Errorf("marshal token data: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("create cipher: %w", err)
	}

	blockSize := block.BlockSize()
	padding := blockSize - len(plaintext)%blockSize
	padded := make([]byte, len(plaintext)+padding)
	copy(padded, plaintext)
	for i := len(plaintext); i < len(padded); i++ {
		padded[i] = byte(padding)
	}

	iv := make([]byte, blockSize)
	if _, err := rand.Read(iv); err != nil {
		return "", fmt.Errorf("generate IV: %w", err)
	}

	mode := cipher.NewCBCEncrypter(block, iv)
	encrypted := make([]byte, len(padded))
	mode.CryptBlocks(encrypted, padded)

	envelope := map[string]string{
		"iv":    base64.StdEncoding.EncodeToString(iv),
		"value": base64.StdEncoding.EncodeToString(encrypted),
	}

	envelopeJSON, _ := json.Marshal(envelope)
	return base64.StdEncoding.EncodeToString(envelopeJSON), nil
}

// DecryptToken decrypts an AES-256-CBC token produced by EncryptToken.
func DecryptToken(key []byte, token string) (map[string]interface{}, error) {
	envelopeJSON, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("decode token envelope: %w", err)
	}

	var envelope struct {
		IV    string `json:"iv"`
		Value string `json:"value"`
	}
	if err := json.Unmarshal(envelopeJSON, &envelope); err != nil {
		return nil, fmt.Errorf("unmarshal envelope: %w", err)
	}

	iv, err := base64.StdEncoding.DecodeString(envelope.IV)
	if err != nil {
		return nil, fmt.Errorf("decode IV: %w", err)
	}

	encrypted, err := base64.StdEncoding.DecodeString(envelope.Value)
	if err != nil {
		return nil, fmt.Errorf("decode value: %w", err)
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}

	if len(iv) != block.BlockSize() {
		return nil, fmt.Errorf("IV length %d does not match block size %d", len(iv), block.BlockSize())
	}
	if len(encrypted) == 0 || len(encrypted)%block.BlockSize() != 0 {
		return nil, fmt.Errorf("ciphertext length %d is not a multiple of block size %d", len(encrypted), block.BlockSize())
	}

	mode := cipher.NewCBCDecrypter(block, iv)
	decrypted := make([]byte, len(encrypted))
	mode.CryptBlocks(decrypted, encrypted)

	paddingLen := int(decrypted[len(decrypted)-1])
	if paddingLen == 0 || paddingLen > block.BlockSize() {
		return nil, fmt.Errorf("invalid PKCS7 padding value %d", paddingLen)
	}
	for i := len(decrypted) - paddingLen; i < len(decrypted); i++ {
		if decrypted[i] != byte(paddingLen) {
			return nil, fmt.Errorf("inconsistent PKCS7 padding at byte %d", i)
		}
	}
	decrypted = decrypted[:len(decrypted)-paddingLen]

	var result map[string]interface{}
	if err := json.Unmarshal(decrypted, &result); err != nil {
		return nil, fmt.Errorf("unmarshal decrypted data: %w", err)
	}

	return result, nil
}

// GenerateEncryptionKey returns a cryptographically random 32-byte key for AES-256.
func GenerateEncryptionKey() ([]byte, error) {
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	return key, nil
}
