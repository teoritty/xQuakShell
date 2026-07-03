package usecase

import (
	"encoding/base64"
	"encoding/json"
)

func encodeTunnelData(sessionID, tunnelID string, data []byte) ([]byte, error) {
	payload := map[string]string{
		"sessionId":    sessionID,
		"tunnelId":     tunnelID,
		"dataBase64":   base64.StdEncoding.EncodeToString(data),
	}
	return json.Marshal(payload)
}

func decodeTunnelData(dataBase64 string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(dataBase64)
}
